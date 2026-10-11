package match

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"game/services/internal/actormsg"
	"game/services/internal/names"

	"gonet"
	"gonet/services/launcher"
)

const (
	spawnTimeout  = 5 * time.Second
	phaseStarting = "starting"
	maxCapacity   = 8
)

// Name 是全服房间目录的别名。全服只有一个，不加 nodeid。
const Name = names.Match

// node 是本进程的 nodeid，用来给房间号加前缀。
var node uint64

// SpawnRoom 由进程入口装上。目录只在开场时拉起场景，场景结束再通知目录。两边不互相引用。
var SpawnRoom func(id string, mode, capacity int, seats []string) gonet.ActorContextInterface

// Bind 记下本进程 nodeid，只用于房间号前缀。目录别名是全服的 Name。
func Bind(nodeID uint64) {
	node = nodeID
}

type record struct {
	mode     int
	capacity int
	seats    []string
	phase    string
	pid      uint64
}

// Actor 管全部等人的房间：创建、加入、离开、是否人满。一场开场之后才有场景 actor。
type Actor struct {
	gonet.ActorContext
	rooms  map[string]*record
	byRole map[string]string
	nextID uint64
}

func New() *Actor {
	return &Actor{
		rooms:  make(map[string]*record),
		byRole: make(map[string]string),
	}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "match", "pid", a.Self(), "name", a.SelfName())
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	var pids []uint64
	for _, rec := range a.rooms {
		if rec.pid != 0 {
			pids = append(pids, rec.pid)
		}
	}
	a.rooms = make(map[string]*record)
	a.byRole = make(map[string]string)
	launcher.DropWaits(a.Self(), gonet.ErrDead)
	slog.Info("service stopped", "service", "match", "pid", a.Self(), "name", a.SelfName(), "scenes", len(pids))
	if len(pids) == 0 {
		return
	}
	go func() {
		for _, pid := range pids {
			_ = gonet.StopActor(pid)
		}
	}()
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchCreate, func() gonet.MessageInterface { return &actormsg.MatchCreateMsg{} }, a.onCreate); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchJoin, func() gonet.MessageInterface { return &actormsg.MatchJoinMsg{} }, a.onJoin); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchLeave, func() gonet.MessageInterface { return &actormsg.MatchLeaveMsg{} }, a.onLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchStart, func() gonet.MessageInterface { return &actormsg.MatchStartMsg{} }, a.onStart); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchDone, func() gonet.MessageInterface { return &actormsg.MatchDoneMsg{} }, a.onDone); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchRelease, func() gonet.MessageInterface { return &actormsg.MatchReleaseMsg{} }, a.onRelease); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse)
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, _ := e.Msg.(*gonet.CallResponse)
	if m == nil {
		return
	}
	launcher.DispatchResponse(m)
}

func (a *Actor) onCreate(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchCreateMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	if m.Mode < 0 {
		e.Reply(matchView(actormsg.MatchView{Err: "模式无效"}))
		return
	}
	if m.Capacity < 1 || m.Capacity > maxCapacity {
		e.Reply(matchView(actormsg.MatchView{Err: "人数无效"}))
		return
	}
	if _, ok := a.byRole[m.RoleID]; ok {
		e.Reply(matchView(actormsg.MatchView{Err: "已经在房间"}))
		return
	}
	a.nextID++
	id := fmt.Sprintf("%d_%d", node, a.nextID)
	rec := &record{
		mode:     m.Mode,
		capacity: m.Capacity,
		seats:    []string{m.RoleID},
		phase:    actormsg.PhaseMatchWait,
	}
	a.rooms[id] = rec
	a.byRole[m.RoleID] = id
	slog.Info("room waiting", "room", id, "mode", m.Mode, "capacity", m.Capacity, "roleid", m.RoleID)
	e.Reply(rec.view(id))
}

func (a *Actor) onJoin(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchJoinMsg)
	if m == nil || m.RoleID == "" || m.RoomID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	if _, ok := a.byRole[m.RoleID]; ok {
		e.Reply(matchView(actormsg.MatchView{Err: "已经在房间"}))
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil {
		e.Reply(matchView(actormsg.MatchView{Err: "房间不存在"}))
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(matchView(actormsg.MatchView{Err: "正在开场"}))
		return
	case actormsg.PhaseMatchPlay:
		e.Reply(matchView(actormsg.MatchView{Err: "已经开场"}))
		return
	}
	if len(rec.seats) >= rec.capacity {
		e.Reply(matchView(actormsg.MatchView{Err: "房间已满"}))
		return
	}
	rec.seats = append(rec.seats, m.RoleID)
	a.byRole[m.RoleID] = m.RoomID
	a.notify(rec, m.RoomID, actormsg.KindMatchJoin, m.RoleID, m.RoleID)
	e.Reply(rec.view(m.RoomID))
}

func (a *Actor) onLeave(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchLeaveMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	id := a.byRole[m.RoleID]
	rec := a.rooms[id]
	if id == "" || rec == nil {
		e.Reply(matchView(actormsg.MatchView{Err: "不在房间"}))
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(matchView(actormsg.MatchView{Err: "正在开场"}))
		return
	case actormsg.PhaseMatchPlay:
		e.Reply(matchView(actormsg.MatchView{Err: "已经开场"}))
		return
	}
	rec.seats = removeSeat(rec.seats, m.RoleID)
	delete(a.byRole, m.RoleID)
	if len(rec.seats) == 0 {
		delete(a.rooms, id)
		slog.Info("room dropped", "room", id)
		e.Reply(matchView(actormsg.MatchView{}))
		return
	}
	a.notify(rec, id, actormsg.KindMatchLeave, m.RoleID, m.RoleID)
	e.Reply(matchView(actormsg.MatchView{}))
}

func (a *Actor) onStart(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchStartMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	id := a.byRole[m.RoleID]
	rec := a.rooms[id]
	if id == "" || rec == nil {
		e.Reply(matchView(actormsg.MatchView{Err: "不在房间"}))
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(matchView(actormsg.MatchView{Err: "正在开场"}))
		return
	case actormsg.PhaseMatchPlay:
		e.Reply(matchView(actormsg.MatchView{Err: "已经开场"}))
		return
	}
	if len(rec.seats) < rec.capacity {
		e.Reply(matchView(actormsg.MatchView{Err: "人还没满"}))
		return
	}
	if SpawnRoom == nil {
		e.Reply(matchView(actormsg.MatchView{Err: "玩法场景还没接上"}))
		return
	}
	impl := SpawnRoom(id, rec.mode, rec.capacity, copySeats(rec.seats))
	if impl == nil {
		e.Reply(matchView(actormsg.MatchView{Err: "玩法场景还没接上"}))
		return
	}
	rec.phase = phaseStarting
	err := launcher.NewService(a.Self(), impl, names.Room(id), spawnTimeout, func(res launcher.ServiceResult) {
		a.finishStart(id, e, res)
	})
	if err != nil {
		rec.phase = actormsg.PhaseMatchWait
		e.Reply(matchView(actormsg.MatchView{Err: err.Error()}))
	}
}

func (a *Actor) finishStart(id string, reply gonet.Envelope, res launcher.ServiceResult) {
	rec := a.rooms[id]
	if rec == nil || rec.phase != phaseStarting {
		if res.PID != 0 {
			go gonet.StopActor(res.PID)
		}
		reply.Reply(matchView(actormsg.MatchView{Err: "房间不存在"}))
		return
	}
	if res.Err != nil || res.PID == 0 {
		rec.phase = actormsg.PhaseMatchWait
		text := "玩法场景没有起来"
		if res.Err != nil {
			text = res.Err.Error()
		}
		reply.Reply(matchView(actormsg.MatchView{Err: text}))
		return
	}
	rec.phase = actormsg.PhaseMatchPlay
	rec.pid = res.PID
	slog.Info("room started", "room", id, "pid", res.PID, "seats", len(rec.seats))
	a.notify(rec, id, actormsg.KindMatchStart, "", "")
	reply.Reply(rec.view(id))
}

func (a *Actor) onDone(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchDoneMsg)
	if m == nil || m.RoomID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil {
		e.Reply(matchView(actormsg.MatchView{}))
		return
	}
	for _, seat := range rec.seats {
		if a.byRole[seat] == m.RoomID {
			delete(a.byRole, seat)
		}
	}
	delete(a.rooms, m.RoomID)
	slog.Info("room ended", "room", m.RoomID)
	e.Reply(matchView(actormsg.MatchView{}))
}

func (a *Actor) onRelease(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.MatchReleaseMsg)
	if m == nil || m.RoomID == "" || m.RoleID == "" {
		e.Reply(matchView(actormsg.MatchView{Err: "参数无效"}))
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil || rec.phase != actormsg.PhaseMatchPlay || a.byRole[m.RoleID] != m.RoomID {
		e.Reply(matchView(actormsg.MatchView{Err: "不在房间"}))
		return
	}
	rec.seats = removeSeat(rec.seats, m.RoleID)
	delete(a.byRole, m.RoleID)
	e.Reply(matchView(actormsg.MatchView{}))
}

func (a *Actor) notify(rec *record, id, kind, who, skip string) {
	msg := &actormsg.MatchNotifyMsg{
		Kind:     kind,
		RoomID:   id,
		RoleID:   who,
		Mode:     rec.mode,
		Capacity: rec.capacity,
		Seats:    copySeats(rec.seats),
		Phase:    clientPhase(rec.phase),
	}
	msg.SetCmd(actormsg.CmdMatchNotify)
	for _, seat := range rec.seats {
		if seat == "" || seat == skip {
			continue
		}
		if err := gonet.SendName(names.Role(seat), msg); err != nil && !gone(err) {
			slog.Warn("room notify", "room", id, "roleid", seat, "kind", kind, "err", err)
		}
	}
}

func (rec *record) view(id string) *actormsg.MatchView {
	return matchView(actormsg.MatchView{
		RoomID:   id,
		Mode:     rec.mode,
		Capacity: rec.capacity,
		Seats:    copySeats(rec.seats),
		Phase:    clientPhase(rec.phase),
	})
}

func clientPhase(phase string) string {
	if phase == actormsg.PhaseMatchPlay {
		return actormsg.PhaseMatchPlay
	}
	return actormsg.PhaseMatchWait
}

func copySeats(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func removeSeat(seats []string, roleID string) []string {
	out := make([]string, 0, len(seats))
	for _, seat := range seats {
		if seat != roleID {
			out = append(out, seat)
		}
	}
	return out
}

func gone(err error) bool {
	return errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead)
}

func matchView(v actormsg.MatchView) *actormsg.MatchView {
	v.SetCmd(actormsg.CmdMatchView)
	return &v
}
