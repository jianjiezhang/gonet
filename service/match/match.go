package match

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gonet"
	"gonet/services/launcher"
)

const spawnTimeout = 5 * time.Second

// Name 是本进程房间目录的别名。Bind 之前为空。
var Name string

// node 是本进程的 nodeid，用来给房间号加前缀。
var node uint64

// SpawnRoom 由 service/room 在 init 时装上。目录只在开场时拉起场景，场景结束再通知目录。两边不能互相引用。
var SpawnRoom func(id string, mode, capacity int, seats []string) gonet.ActorContextInterface

// Bind 按 harbor 分配的 nodeid 确定房间目录别名。
func Bind(nodeID uint64) {
	node = nodeID
	Name = gonet.ServiceAlias("match", nodeID)
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
	if err := gonet.RegisterCmd(a, CmdCreate, func() gonet.MessageInterface { return &CreateMsg{} }, a.onCreate); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdJoin, func() gonet.MessageInterface { return &JoinMsg{} }, a.onJoin); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdLeave, func() gonet.MessageInterface { return &LeaveMsg{} }, a.onLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdStart, func() gonet.MessageInterface { return &StartMsg{} }, a.onStart); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdDone, func() gonet.MessageInterface { return &DoneMsg{} }, a.onDone); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdRelease, func() gonet.MessageInterface { return &ReleaseMsg{} }, a.onRelease); err != nil {
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
	m, _ := e.Msg.(*CreateMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if m.Mode < 0 {
		e.Reply(&View{Err: "模式无效"})
		return
	}
	if m.Capacity < 1 || m.Capacity > maxCapacity {
		e.Reply(&View{Err: "人数无效"})
		return
	}
	if _, ok := a.byRole[m.RoleID]; ok {
		e.Reply(&View{Err: "已经在房间"})
		return
	}
	a.nextID++
	id := fmt.Sprintf("%d_%d", node, a.nextID)
	rec := &record{
		mode:     m.Mode,
		capacity: m.Capacity,
		seats:    []string{m.RoleID},
		phase:    PhaseWait,
	}
	a.rooms[id] = rec
	a.byRole[m.RoleID] = id
	slog.Info("room waiting", "room", id, "mode", m.Mode, "capacity", m.Capacity, "roleid", m.RoleID)
	e.Reply(rec.view(id))
}

func (a *Actor) onJoin(e gonet.Envelope) {
	m, _ := e.Msg.(*JoinMsg)
	if m == nil || m.RoleID == "" || m.RoomID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if _, ok := a.byRole[m.RoleID]; ok {
		e.Reply(&View{Err: "已经在房间"})
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil {
		e.Reply(&View{Err: "房间不存在"})
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(&View{Err: "正在开场"})
		return
	case PhasePlay:
		e.Reply(&View{Err: "已经开场"})
		return
	}
	if len(rec.seats) >= rec.capacity {
		e.Reply(&View{Err: "房间已满"})
		return
	}
	rec.seats = append(rec.seats, m.RoleID)
	a.byRole[m.RoleID] = m.RoomID
	a.notify(rec, m.RoomID, KindJoin, m.RoleID, m.RoleID)
	e.Reply(rec.view(m.RoomID))
}

func (a *Actor) onLeave(e gonet.Envelope) {
	m, _ := e.Msg.(*LeaveMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	id := a.byRole[m.RoleID]
	rec := a.rooms[id]
	if id == "" || rec == nil {
		e.Reply(&View{Err: "不在房间"})
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(&View{Err: "正在开场"})
		return
	case PhasePlay:
		e.Reply(&View{Err: "已经开场"})
		return
	}
	rec.seats = removeSeat(rec.seats, m.RoleID)
	delete(a.byRole, m.RoleID)
	if len(rec.seats) == 0 {
		delete(a.rooms, id)
		slog.Info("room dropped", "room", id)
		e.Reply(&View{})
		return
	}
	a.notify(rec, id, KindLeave, m.RoleID, m.RoleID)
	e.Reply(&View{})
}

func (a *Actor) onStart(e gonet.Envelope) {
	m, _ := e.Msg.(*StartMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	id := a.byRole[m.RoleID]
	rec := a.rooms[id]
	if id == "" || rec == nil {
		e.Reply(&View{Err: "不在房间"})
		return
	}
	switch rec.phase {
	case phaseStarting:
		e.Reply(&View{Err: "正在开场"})
		return
	case PhasePlay:
		e.Reply(&View{Err: "已经开场"})
		return
	}
	if len(rec.seats) < rec.capacity {
		e.Reply(&View{Err: "人还没满"})
		return
	}
	if SpawnRoom == nil {
		e.Reply(&View{Err: "玩法场景还没接上"})
		return
	}
	impl := SpawnRoom(id, rec.mode, rec.capacity, copySeats(rec.seats))
	if impl == nil {
		e.Reply(&View{Err: "玩法场景还没接上"})
		return
	}
	rec.phase = phaseStarting
	err := launcher.NewService(a.Self(), impl, roomAlias(id), spawnTimeout, func(res launcher.ServiceResult) {
		a.finishStart(id, e, res)
	})
	if err != nil {
		rec.phase = PhaseWait
		e.Reply(&View{Err: err.Error()})
	}
}

func (a *Actor) finishStart(id string, reply gonet.Envelope, res launcher.ServiceResult) {
	rec := a.rooms[id]
	if rec == nil || rec.phase != phaseStarting {
		if res.PID != 0 {
			go gonet.StopActor(res.PID)
		}
		reply.Reply(&View{Err: "房间不存在"})
		return
	}
	if res.Err != nil || res.PID == 0 {
		rec.phase = PhaseWait
		text := "玩法场景没有起来"
		if res.Err != nil {
			text = res.Err.Error()
		}
		reply.Reply(&View{Err: text})
		return
	}
	rec.phase = PhasePlay
	rec.pid = res.PID
	slog.Info("room started", "room", id, "pid", res.PID, "seats", len(rec.seats))
	a.notify(rec, id, KindStart, "", "")
	reply.Reply(rec.view(id))
}

func (a *Actor) onDone(e gonet.Envelope) {
	m, _ := e.Msg.(*DoneMsg)
	if m == nil || m.RoomID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil {
		e.Reply(&View{})
		return
	}
	for _, seat := range rec.seats {
		if a.byRole[seat] == m.RoomID {
			delete(a.byRole, seat)
		}
	}
	delete(a.rooms, m.RoomID)
	slog.Info("room ended", "room", m.RoomID)
	e.Reply(&View{})
}

func (a *Actor) onRelease(e gonet.Envelope) {
	m, _ := e.Msg.(*ReleaseMsg)
	if m == nil || m.RoomID == "" || m.RoleID == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	rec := a.rooms[m.RoomID]
	if rec == nil || rec.phase != PhasePlay || a.byRole[m.RoleID] != m.RoomID {
		e.Reply(&View{Err: "不在房间"})
		return
	}
	rec.seats = removeSeat(rec.seats, m.RoleID)
	delete(a.byRole, m.RoleID)
	e.Reply(&View{})
}

func (a *Actor) notify(rec *record, id, kind, who, skip string) {
	msg := &NotifyMsg{
		Kind:     kind,
		RoomID:   id,
		RoleID:   who,
		Mode:     rec.mode,
		Capacity: rec.capacity,
		Seats:    copySeats(rec.seats),
		Phase:    clientPhase(rec.phase),
	}
	msg.SetCmd(CmdNotify)
	for _, seat := range rec.seats {
		if seat == "" || seat == skip {
			continue
		}
		if err := gonet.SendName(roleAlias(seat), msg); err != nil && !gone(err) {
			slog.Warn("room notify", "room", id, "roleid", seat, "kind", kind, "err", err)
		}
	}
}

func (rec *record) view(id string) *View {
	return &View{
		RoomID:   id,
		Mode:     rec.mode,
		Capacity: rec.capacity,
		Seats:    copySeats(rec.seats),
		Phase:    clientPhase(rec.phase),
	}
}

func clientPhase(phase string) string {
	if phase == PhasePlay {
		return PhasePlay
	}
	return PhaseWait
}

func roomAlias(id string) string {
	if id == "" {
		return ""
	}
	return "room/" + id
}

func roleAlias(roleID string) string {
	if roleID == "" {
		return ""
	}
	return "role/" + roleID
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
