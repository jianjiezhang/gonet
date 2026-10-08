package watchdog

import (
	"errors"
	"log/slog"
	"net"
	"time"

	"game/config"
	"game/lib/net"
	"game/lib/packet"
	"game/service/role"
	"protocol/client"

	"gonet"
	"gonet/services/launcher"
)

// Name 是本进程 watchdog 的别名。Bind 之前为空。
var Name string

// Bind 按 harbor 分配的 nodeid 确定本进程的 watchdog 别名。
func Bind(nodeID uint64) {
	Name = gonet.ServiceAlias("watchdog", nodeID)
}

type session struct {
	pid  uint64
	link uint64
}

type loginWait struct {
	conn   net.Conn
	roleID string
	old    uint64
	remote bool
}

// Actor 只处理接入策略：等 login、踢旧号、按 roleid 建玩家。Listen/Accept 在独立协程。
type Actor struct {
	gonet.ActorContext
	sessions  map[string]session
	calls     map[uint64]*loginWait
	loggingIn map[string]struct{}
	retries   map[string]int
}

func New() *Actor {
	return &Actor{
		sessions:  make(map[string]session),
		calls:     make(map[uint64]*loginWait),
		loggingIn: make(map[string]struct{}),
		retries:   make(map[string]int),
	}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "watchdog", "pid", a.Self(), "name", a.SelfName())
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	return StartAccept(config.Get().Addr())
}

func (a *Actor) onPing(e gonet.Envelope) {
	slog.Info("pong", "pid", a.Self(), "alias", a.SelfName(), "service", "watchdog")
	e.Reply("pong")
}

func (a *Actor) onSocketOpen(e gonet.Envelope) {
	m, ok := e.Msg.(*socketOpenMsg)
	if !ok || m.Conn == nil {
		slog.Error("socket.open 无效")
		releasePending()
		return
	}
	slog.Info("client connected, wait login", "remote", m.Conn.RemoteAddr())
	go waitLogin(m.Conn)
}

func (a *Actor) onLogin(e gonet.Envelope) {
	m, ok := e.Msg.(*loginConnMsg)
	if !ok || m.Conn == nil || m.RoleID == "" {
		if m != nil && m.Conn != nil {
			m.Conn.Close()
		}
		return
	}
	exists, err := role.HasRole(m.RoleID)
	if err != nil {
		slog.Error("exists role", "roleid", m.RoleID, "err", err)
		m.Conn.Close()
		return
	}
	if want := config.Get().LoginToken; want != "" && m.Token != want {
		slog.Info("login auth failed", "roleid", m.RoleID, "remote", m.Conn.RemoteAddr())
		_ = client.NewLoginErr(client.AuthFail).Send(func(cmd string, body any) error {
			return writeConnCmd(m.Conn, cmd, body)
		})
		m.Conn.Close()
		return
	}
	if !exists {
		if err := role.CreateRole(m.RoleID); err != nil {
			slog.Error("create role", "roleid", m.RoleID, "err", err)
			m.Conn.Close()
			return
		}
	}
	if _, busy := a.loggingIn[m.RoleID]; busy {
		slog.Info("login busy", "roleid", m.RoleID)
		m.Conn.Close()
		return
	}
	a.loggingIn[m.RoleID] = struct{}{}
	old, err := gonet.Query(role.Alias(m.RoleID))
	if err != nil && !errors.Is(err, gonet.ErrUnknownAlias) {
		slog.Error("query role", "roleid", m.RoleID, "err", err)
		a.failLogin(m.Conn, m.RoleID)
		return
	}
	if err == nil {
		a.startKick(m.Conn, m.RoleID, old)
		return
	}
	a.startSpawn(m.Conn, m.RoleID)
}

func (a *Actor) startKick(conn net.Conn, roleID string, old uint64) {
	slog.Info("kick online", "roleid", roleID, "pid", old)
	delete(a.sessions, roleID)
	kick := &role.KickMsg{BaseMessage: gonet.BaseMessage{Cmd: "kick"}}
	sess, err := a.Call(old, 2*time.Second, kick)
	if err != nil {
		if !errors.Is(err, gonet.ErrDead) {
			slog.Error("kick call", "roleid", roleID, "pid", old, "err", err)
			a.failLogin(conn, roleID)
			return
		}
		a.startSpawn(conn, roleID)
		return
	}
	a.calls[sess] = &loginWait{conn: conn, roleID: roleID, old: old}
}

func (a *Actor) startRemoteKick(conn net.Conn, roleID string) {
	slog.Info("kick remote", "roleid", roleID, "alias", role.Alias(roleID))
	kick := &role.KickMsg{BaseMessage: gonet.BaseMessage{Cmd: "kick"}}
	sess, err := a.CallName(role.Alias(roleID), 2*time.Second, kick)
	if err != nil {
		if errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead) {
			a.startSpawn(conn, roleID)
			return
		}
		slog.Error("kick remote", "roleid", roleID, "err", err)
		a.failLogin(conn, roleID)
		return
	}
	a.calls[sess] = &loginWait{conn: conn, roleID: roleID, remote: true}
}

func (a *Actor) startSpawn(conn net.Conn, roleID string) {
	r := role.New()
	err := launcher.NewService(a.Self(), r, role.Alias(roleID), 5*time.Second, func(res launcher.ServiceResult) {
		a.afterSpawn(conn, roleID, res)
	})
	if err != nil {
		slog.Error("new role", "roleid", roleID, "err", err)
		a.failLogin(conn, roleID)
	}
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, ok := e.Msg.(*gonet.CallResponse)
	if !ok {
		return
	}
	if launcher.DispatchResponse(m) {
		return
	}
	w := a.calls[m.Session]
	delete(a.calls, m.Session)
	if w == nil {
		return
	}
	a.afterKick(w, m)
}

func (a *Actor) afterKick(w *loginWait, m *gonet.CallResponse) {
	if w.remote {
		if m.Err != nil && !errors.Is(m.Err, gonet.ErrDead) && !errors.Is(m.Err, gonet.ErrUnknownAlias) {
			slog.Info("kick remote", "roleid", w.roleID, "err", m.Err)
			a.failLogin(w.conn, w.roleID)
			return
		}
		a.scheduleSpawn(w.conn, w.roleID)
		return
	}
	if m.Err != nil && !errors.Is(m.Err, gonet.ErrDead) {
		slog.Info("kick notify", "roleid", w.roleID, "pid", w.old, "err", m.Err)
		a.failLogin(w.conn, w.roleID)
		return
	}
	self := a.Self()
	go func() {
		err := gonet.StopActor(w.old)
		msg := &kickDoneMsg{Conn: w.conn, RoleID: w.roleID, Old: w.old, Err: err}
		msg.SetCmd(cmdKickDone)
		_ = gonet.SendMemory(self, msg)
	}()
}

func (a *Actor) onKickDone(e gonet.Envelope) {
	m, ok := e.Msg.(*kickDoneMsg)
	if !ok {
		return
	}
	if m.Err != nil && !errors.Is(m.Err, gonet.ErrDead) {
		slog.Info("stop old role", "roleid", m.RoleID, "pid", m.Old, "err", m.Err)
	}
	if _, ok := a.loggingIn[m.RoleID]; !ok {
		return
	}
	a.startSpawn(m.Conn, m.RoleID)
}

func (a *Actor) afterSpawn(conn net.Conn, roleID string, res launcher.ServiceResult) {
	if errors.Is(res.Err, gonet.ErrAliasTaken) {
		if a.retries[roleID] >= 2 {
			slog.Error("role alias taken", "roleid", roleID, "alias", role.Alias(roleID))
			a.failLogin(conn, roleID)
			return
		}
		a.retries[roleID]++
		a.startRemoteKick(conn, roleID)
		return
	}
	if res.Err != nil {
		slog.Error("new role", "roleid", roleID, "err", res.Err)
		a.failLogin(conn, roleID)
		return
	}
	link := role.NewLink()
	if err := role.AttachConn(res.PID, conn, link); err != nil {
		slog.Error("attach conn", "roleid", roleID, "pid", res.PID, "err", err)
		go gonet.StopActor(res.PID)
		a.failLogin(conn, roleID)
		return
	}
	delete(a.loggingIn, roleID)
	delete(a.retries, roleID)
	a.sessions[roleID] = session{pid: res.PID, link: link}
	slog.Info("login ok", "pid", res.PID, "roleid", roleID, "remote", conn.RemoteAddr())
	_ = client.NewLoginOK().Send(func(cmd string, body any) error {
		return writeConnCmd(conn, cmd, body)
	})
}

func (a *Actor) scheduleSpawn(conn net.Conn, roleID string) {
	msg := &retryLoginMsg{Conn: conn, RoleID: roleID}
	if _, err := a.Timeout(300*time.Millisecond, msg); err != nil {
		slog.Error("retry login", "roleid", roleID, "err", err)
		a.failLogin(conn, roleID)
	}
}

func (a *Actor) onRetryLogin(e gonet.Envelope) {
	m, ok := e.Msg.(*retryLoginMsg)
	if !ok || m.Conn == nil || m.RoleID == "" {
		return
	}
	if _, ok := a.loggingIn[m.RoleID]; !ok {
		m.Conn.Close()
		return
	}
	a.startSpawn(m.Conn, m.RoleID)
}

func (a *Actor) failLogin(conn net.Conn, roleID string) {
	delete(a.loggingIn, roleID)
	delete(a.retries, roleID)
	if conn != nil {
		conn.Close()
	}
}

func (a *Actor) onSocketClose(e gonet.Envelope) {
	m, ok := e.Msg.(*socketCloseMsg)
	if !ok || m.Alias == "" || m.Link == 0 {
		return
	}
	s, ok := a.sessions[m.Alias]
	if !ok || s.link != m.Link {
		return
	}
	delete(a.sessions, m.Alias)
	pid, alias := s.pid, m.Alias
	go func() {
		if err := gonet.StopActor(pid); err != nil {
			if errors.Is(err, gonet.ErrDead) {
				slog.Info("client disconnected", "pid", pid, "alias", alias)
				return
			}
			slog.Info("stop role", "pid", pid, "alias", alias, "err", err)
			return
		}
		slog.Info("client disconnected", "pid", pid, "alias", alias)
	}()
}

func (a *Actor) Term() {
	n := len(a.sessions)
	slog.Info("service stopping", "service", "watchdog", "pid", a.Self(), "name", a.SelfName(), "sessions", n)
	_ = StopAccept()
	for _, s := range a.sessions {
		_ = gonet.StopActor(s.pid)
	}
	a.sessions = make(map[string]session)
	a.calls = make(map[uint64]*loginWait)
	a.loggingIn = make(map[string]struct{})
	a.retries = make(map[string]int)
	launcher.DropWaits(a.Self(), gonet.ErrDead)
	slog.Info("service stopped", "service", "watchdog", "pid", a.Self(), "name", a.SelfName())
}

func writeConnCmd(conn interface{ Write([]byte) (int, error) }, cmd string, payload any) error {
	slog.Info(gamenet.FrameLog("send", cmd, payload))
	return gamenet.WriteMsg(conn, cmd, payload)
}

func waitLogin(conn net.Conn) {
	defer gonet.EnterService(Name)()
	defer releasePending()
	msg, err := gamenet.ReadMsg(conn)
	if err != nil {
		if !packet.IsGone(err) {
			slog.Info("login wait closed", "remote", conn.RemoteAddr(), "err", err)
		}
		conn.Close()
		return
	}
	slog.Info(gamenet.FrameLog("recv", gamenet.Cmd(msg), msg))
	login, ok := msg.(*loginConnMsg)
	if !ok || login.RoleID == "" {
		slog.Info("expect login", "remote", conn.RemoteAddr(), "cmd", gamenet.Cmd(msg))
		conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	login.Conn = conn
	if err := gonet.SendMemoryName(Name, login); err != nil {
		slog.Error("notify watchdog login", "err", err)
		conn.Close()
	}
}
