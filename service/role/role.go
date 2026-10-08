package role

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"game/config"
	"game/lib/net"
	"game/lib/packet"
	"game/service/onlinemgr"
	"protocol/client"

	"gonet"
)

const (
	heartbeatTimeout       = 15 * time.Second
	heartbeatCheckInterval = 5 * time.Second
	writeTimeout           = 5 * time.Second
)

func watchdogAlias() string {
	return gonet.ServiceAlias("watchdog", gonet.ClusterNode())
}

var nextID atomic.Uint64
var nextLink atomic.Uint64

// Actor 是玩家 actor。
type Actor struct {
	gonet.ActorContext
	data          *Data
	conn          net.Conn
	link          uint64
	lastHeartbeat atomic.Int64
	dirty         bool
	announced     bool
	saveWG        sync.WaitGroup
	friendWaits   map[uint64]friendWait
	listBuilds    map[uint64]*listBuild
	onlineWaits   map[uint64]uint64
	guildWaits    map[uint64]guildWait
	roomWaits     map[uint64]roomWait
	roomID        string
	roomPlay      bool
	nextBuild     uint64
}

func New() *Actor {
	return &Actor{}
}

func (a *Actor) BindConn(c net.Conn) {
	a.conn = c
}

// NewLink 分配一次接入编号。同一 roleid 再次登录会换成新的。
func NewLink() uint64 {
	return nextLink.Add(1)
}

// AttachConn 在 role 就绪后把连接和接入编号送进它的 mailbox。连接只走 SendMemory。
func AttachConn(pid uint64, conn net.Conn, link uint64) error {
	if conn == nil || link == 0 {
		return gonet.ErrNilMessage
	}
	msg := &attachConnMsg{Conn: conn, Link: link}
	msg.SetCmd(cmdAttachConn)
	return gonet.SendMemory(pid, msg)
}

func (a *Actor) onAttachConn(e gonet.Envelope) {
	m, ok := e.Msg.(*attachConnMsg)
	if !ok || m.Conn == nil || m.Link == 0 {
		return
	}
	if a.conn != nil {
		_ = m.Conn.Close()
		return
	}
	a.link = m.Link
	a.BindConn(m.Conn)
	a.noteLogin()
	go a.readLoop()
}

func (a *Actor) sameLink(msg gonet.MessageInterface) bool {
	m, ok := msg.(interface{ LinkID() uint64 })
	return ok && a.link != 0 && m.LinkID() == a.link
}

// NextAlias 生成本进程下一个角色号：nodeid*1000000 + 本进程自增。
// 别名仍是 role/角色号。登录带来的角色号不走这里。
func NextAlias() string {
	id := nextID.Add(1)
	return strconv.FormatUint(gonet.ClusterNode()*1_000_000+id, 10)
}

func (a *Actor) Init() error {
	data, err := LoadData(RoleID(a.SelfName()))
	if err != nil {
		return err
	}
	a.friendWaits = make(map[uint64]friendWait)
	a.listBuilds = make(map[uint64]*listBuild)
	a.onlineWaits = make(map[uint64]uint64)
	a.guildWaits = make(map[uint64]guildWait)
	a.roomWaits = make(map[uint64]roomWait)
	a.data = data
	a.data.Base.bindChange(func() {
		a.markDirty()
		a.data.Missions.Notify(config.MissionKindLevel, a.data.Base.Level())
	})
	a.data.Missions.bindChange(a.markDirty)
	a.data.Missions.Bootstrap(a.data.Base.Level())
	slog.Info("service started", "service", "role", "pid", a.Self(), "name", a.SelfName(),
		"level", a.data.Base.Level(), "rolename", a.data.Base.Name(), "gender", a.data.Base.Gender())

	if err := a.RegisterCmds(); err != nil {
		return err
	}
	a.touchHeartbeat()
	if err := a.armHeartbeatCheck(); err != nil {
		return err
	}
	if err := a.startPersist(); err != nil {
		return err
	}
	if a.conn != nil {
		a.noteLogin()
		go a.readLoop()
	}
	return nil
}

func (a *Actor) Term() {
	a.noteRoomOff()
	slog.Info("service stopped", "service", "role", "pid", a.Self(), "name", a.SelfName())
	a.noteLogout()
	a.saveWG.Wait()
	a.flushSave(false)
	if a.conn != nil {
		_ = a.conn.Close()
	}
}

func (a *Actor) onPing(e gonet.Envelope) {
	slog.Info("pong", "pid", a.Self(), "alias", a.SelfName(), "service", "role")
	e.Reply("pong")
	_ = a.write(client.NewPong())
}

func (a *Actor) onHeartbeat(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.touchHeartbeat()
	e.Reply("ok")
	_ = a.write(client.NewHeartbeatOK())
}

func (a *Actor) onKick(e gonet.Envelope) {
	_ = a.write(client.NewKick())
	e.Reply("ok")
	a.Exit()
}

func (a *Actor) onMissionList(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	e.Reply("ok")
	_ = a.write(client.NewMissionList(a.data.Missions.List()))
}

func (a *Actor) onMissionFinish(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	msg, _ := e.Msg.(*MissionFinishMsg)
	id := 0
	if msg != nil {
		id = msg.ID
	}
	delta, err := a.data.Missions.Claim(id, a.data.Base.Level())
	if err != nil {
		e.Reply(err.Error())
		_ = a.write(client.NewErr(client.MissionFinish, err.Error()))
		return
	}
	if delta > 0 {
		_ = a.data.Base.AddLevel(delta)
	}
	e.Reply("ok")
	_ = a.write(client.NewMissionFinishOK(id, delta, a.data.Base.Level(), a.data.Missions.List()))
}

func (a *Actor) onRoleInfo(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	e.Reply("ok")
	_ = a.write(a.roleInfo())
}

func (a *Actor) onSetLevel(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	msg, _ := e.Msg.(*SetLevelMsg)
	lv := 0
	if msg != nil {
		lv = msg.Level
	}
	if err := a.data.Base.SetLevel(lv); err != nil {
		e.Reply(err.Error())
		_ = a.write(client.NewErr(client.SetLevel, err.Error()))
		return
	}
	e.Reply("ok")
	_ = a.write(client.NewSetLevelOK(a.data.Base.Level(), a.data.Missions.List()))
}

func (a *Actor) onHbCheck(e gonet.Envelope) {
	a.onHeartbeatCheck()
	e.Reply("ok")
}

func (a *Actor) touchHeartbeat() {
	a.lastHeartbeat.Store(time.Now().UnixNano())
}

func (a *Actor) armHeartbeatCheck() error {
	msg := &HbCheckMsg{}
	msg.SetCmd(cmdHbCheck)
	_, err := gonet.Timeout(a.Self(), heartbeatCheckInterval, msg)
	return err
}

func (a *Actor) onHeartbeatCheck() {
	last := a.lastHeartbeat.Load()
	if last != 0 && time.Since(time.Unix(0, last)) > heartbeatTimeout {
		slog.Info("heartbeat timeout", "pid", a.Self(), "alias", a.SelfName())
		a.dropConn()
		a.Exit()
		return
	}
	_ = a.armHeartbeatCheck()
}

func (a *Actor) dropConn() {
	if a.conn != nil {
		_ = a.conn.Close()
	}
}

func (a *Actor) noteLogin() {
	if a == nil || a.announced || a.data == nil || a.data.Base == nil {
		return
	}
	a.data.Base.SetLastLoginTime(time.Now().Unix())
	a.markDirty()
	a.announced = true
	a.syncOnline(true)
}

func (a *Actor) noteLogout() {
	if a == nil || !a.announced || a.data == nil || a.data.Base == nil {
		return
	}
	a.data.Base.SetLastLogoutTime(time.Now().Unix())
	a.markDirty()
	a.announced = false
	a.syncOnline(false)
}

func (a *Actor) syncOnline(login bool) {
	name := onlinemgr.Name
	if name == "" {
		return
	}
	id := RoleID(a.SelfName())
	if id == "" {
		return
	}
	var msg gonet.MessageInterface
	if login {
		m := &onlinemgr.LoginMsg{RoleID: id, Alias: a.SelfName(), PID: a.Self()}
		m.SetCmd(onlinemgr.CmdLogin)
		msg = m
	} else {
		m := &onlinemgr.LogoutMsg{RoleID: id, PID: a.Self()}
		m.SetCmd(onlinemgr.CmdLogout)
		msg = m
	}
	if err := gonet.SendName(name, msg); err != nil {
		slog.Warn("sync online", "roleid", id, "login", login, "err", err)
	}
}

func (a *Actor) onOnlinePush(e gonet.Envelope) {
	m, ok := e.Msg.(*onlinemgr.PushMsg)
	if !ok || m.Cmd == "" {
		return
	}
	var payload any
	if m.Data != "" {
		payload = json.RawMessage(m.Data)
	}
	_ = a.writeCmd(m.Cmd, payload)
}

func (a *Actor) roleInfo() client.Out {
	if a == nil || a.data == nil || a.data.Base == nil {
		return client.Out{Cmd: client.RoleInfo, Body: struct{}{}}
	}
	return client.NewRoleInfo(RoleID(a.SelfName()), a.data.Base.Name(), a.data.Base.Level(), a.data.Base.Gender())
}

func (a *Actor) write(msg client.Out) error {
	return msg.Send(a.writeCmd)
}

// logFrame 记下一条玩家协议。心跳几秒一次，收发都不打。
func logFrame(dir, cmd string, payload any) {
	if cmd == client.Heartbeat {
		return
	}
	slog.Info(gamenet.FrameLog(dir, cmd, payload))
}

func (a *Actor) writeCmd(cmd string, payload any) error {
	if a.conn == nil {
		return nil
	}
	logFrame("send", cmd, payload)
	_ = a.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	err := gamenet.WriteMsg(a.conn, cmd, payload)
	if err == nil {
		return nil
	}
	if packet.IsGone(err) {
		a.dropConn()
		return err
	}
	if errors.Is(err, packet.ErrTooLarge) || errors.Is(err, packet.ErrEmpty) {
		slog.Warn("role write packet", "alias", a.SelfName(), "cmd", cmd, "err", err)
		return err
	}
	slog.Info("role write", "alias", a.SelfName(), "cmd", cmd, "err", err)
	a.dropConn()
	return err
}

func (a *Actor) readLoop() {
	alias := a.SelfName()
	link := a.link
	defer gonet.EnterService(alias)()
	defer notifyClose(RoleID(alias), link)
	for {
		msg, err := gamenet.ReadMsg(a.conn)
		if err != nil {
			if errors.Is(err, gamenet.ErrCodec) {
				slog.Warn("role bad packet", "alias", alias, "err", err)
				continue
			}
			if !packet.IsGone(err) {
				slog.Info("role read", "alias", alias, "err", err)
			}
			return
		}
		logFrame("recv", gamenet.Cmd(msg), msg)
		if gamenet.Cmd(msg) == "" {
			slog.Warn("role empty cmd", "alias", alias)
			continue
		}
		if m, ok := msg.(interface{ SetLink(uint64) }); ok {
			m.SetLink(link)
		}
		if err := gonet.SendName(alias, msg); err != nil {
			if !errors.Is(err, gonet.ErrDead) && !errors.Is(err, gonet.ErrUnknownAlias) {
				slog.Warn("role dispatch", "alias", alias, "err", err)
			}
			return
		}
	}
}

func notifyClose(alias string, link uint64) {
	if alias == "" || link == 0 {
		return
	}
	payload := struct {
		Alias string `json:"alias"`
		Link  uint64 `json:"link"`
	}{Alias: alias, Link: link}
	msg, err := gonet.Pack("socket.close", payload)
	if err != nil {
		return
	}
	_ = gonet.SendName(watchdogAlias(), msg)
}
