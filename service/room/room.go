package room

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"time"

	"game/service/match"

	"gonet"
)

func init() {
	match.SpawnRoom = func(id string, mode, capacity int, seats []string) gonet.ActorContextInterface {
		return New(id, mode, capacity, seats)
	}
}

const (
	callTimeout = 2 * time.Second
	frameDur    = time.Second / FrameHz
)

// Actor 是一场已经开场的场景。空房间只记座位；模式 4 另有帧循环。
type Actor struct {
	gonet.ActorContext
	id         string
	mode       int
	capacity   int
	seats      []string
	busy       bool
	end        bool
	pendingEnv gonet.Envelope
	kind       string
	who        string
	sess       uint64

	duo     *Duo
	inputs  []Op
	playing bool
}

func New(id string, mode, capacity int, seats []string) *Actor {
	out := make([]string, len(seats))
	copy(out, seats)
	return &Actor{id: id, mode: mode, capacity: capacity, seats: out}
}

func (a *Actor) Init() error {
	if a.id == "" || ID(a.SelfName()) != a.id {
		return errors.New("room: 房间号对不上")
	}
	if len(a.seats) == 0 {
		return errors.New("room: 没有座位")
	}
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	slog.Info("service started", "service", "room", "pid", a.Self(), "name", a.SelfName(), "room", a.id, "mode", a.mode, "seats", len(a.seats))
	if a.mode == ModeDuo {
		if len(a.seats) != 2 {
			return errors.New("room: 模式 4 需要两个人")
		}
		return a.startDuo()
	}
	return nil
}

func (a *Actor) Term() {
	slog.Info("service stopped", "service", "room", "pid", a.Self(), "name", a.SelfName(), "room", a.id)
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, CmdSettle, func() gonet.MessageInterface { return &SettleMsg{} }, a.onSettle); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdLeave, func() gonet.MessageInterface { return &LeaveMsg{} }, a.onLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdOp, func() gonet.MessageInterface { return &OpMsg{} }, a.onOp); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdDead, func() gonet.MessageInterface { return &DeadMsg{} }, a.onDead); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdTick, func() gonet.MessageInterface { return &TickMsg{} }, a.onTick); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse)
}

func (a *Actor) startDuo() error {
	seed := randomSeed()
	a.duo = NewDuo(seed, a.seats)
	a.inputs = make([]Op, len(a.seats))
	a.playing = true
	a.pushBegin()
	return a.armTick()
}

func (a *Actor) armTick() error {
	msg := &TickMsg{}
	msg.SetCmd(CmdTick)
	_, err := a.Timeout(frameDur, msg)
	return err
}

func (a *Actor) onOp(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if !a.playing || a.duo == nil || a.duo.Done() {
		e.Reply(&Reply{Err: "还没开打"})
		return
	}
	seat := a.duo.SeatOf(m.RoleID)
	if seat < 0 {
		e.Reply(&Reply{Err: "不在这场"})
		return
	}
	a.inputs[seat] = sanitizeOp(Op{Ax: m.Ax, Ay: m.Ay, Dash: m.Dash})
	e.Reply(&Reply{})
}

func (a *Actor) onDead(e gonet.Envelope) {
	m, _ := e.Msg.(*DeadMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if !a.playing || a.duo == nil || a.duo.Done() || a.end {
		e.Reply(&Reply{Err: "还没开打"})
		return
	}
	if m.Frame <= 0 || m.Frame > a.duo.Frame {
		e.Reply(&Reply{Err: "帧号无效"})
		return
	}
	if a.duo.SeatOf(m.RoleID) < 0 {
		e.Reply(&Reply{Err: "不在这场"})
		return
	}
	ended := a.duo.Bite(m.RoleID)
	e.Reply(&Reply{})
	if ended {
		slog.Info("room bite end", "room", a.id, "frame", m.Frame, "roleid", m.RoleID)
		a.finishPlay()
	}
}

func (a *Actor) onTick(e gonet.Envelope) {
	if !a.playing || a.duo == nil || a.duo.Done() || a.end {
		return
	}
	ops := make([]Op, len(a.inputs))
	copy(ops, a.inputs)
	for i := range a.inputs {
		a.inputs[i].Dash = false
	}
	events := a.duo.Step(ops)
	a.pushFrame(ops, events)
	if a.duo.Done() {
		a.finishPlay()
		return
	}
	if err := a.armTick(); err != nil {
		slog.Error("room tick", "room", a.id, "err", err)
		events = a.duo.ForceFail(ReasonManual)
		a.pushFrame(ops, events)
		a.finishPlay()
	}
}

func (a *Actor) onSettle(e gonet.Envelope) {
	m, _ := e.Msg.(*SettleMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if a.busy {
		e.Reply(&Reply{Err: "正在结算"})
		return
	}
	if !seated(a.seats, m.RoleID) {
		e.Reply(&Reply{Err: "不在这场"})
		return
	}
	if a.playing && a.duo != nil && !a.duo.Done() {
		events := a.duo.ForceFail(ReasonManual)
		ops := make([]Op, len(a.inputs))
		copy(ops, a.inputs)
		a.pushFrame(ops, events)
		a.finishPlayFrom(e)
		return
	}
	a.kind = match.KindSettle
	a.end = true
	a.who = m.RoleID
	a.callMatch(e, &match.DoneMsg{RoomID: a.id}, match.CmdDone)
}

func (a *Actor) onLeave(e gonet.Envelope) {
	m, _ := e.Msg.(*LeaveMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if a.busy {
		e.Reply(&Reply{Err: "正在结算"})
		return
	}
	if !seated(a.seats, m.RoleID) {
		e.Reply(&Reply{Err: "不在这场"})
		return
	}
	if a.playing && a.duo != nil && !a.duo.Done() {
		a.duo.MarkDead(m.RoleID)
		a.seats = removeSeat(a.seats, m.RoleID)
		a.who = m.RoleID
		a.kind = match.KindLeave
		if a.duo.Done() {
			a.finishPlayFrom(e)
			return
		}
		a.end = false
		a.callMatch(e, &match.ReleaseMsg{RoomID: a.id, RoleID: m.RoleID}, match.CmdRelease)
		return
	}
	a.seats = removeSeat(a.seats, m.RoleID)
	a.who = m.RoleID
	a.kind = match.KindLeave
	if len(a.seats) == 0 {
		a.end = true
		a.callMatch(e, &match.DoneMsg{RoomID: a.id}, match.CmdDone)
		return
	}
	a.end = false
	a.callMatch(e, &match.ReleaseMsg{RoomID: a.id, RoleID: m.RoleID}, match.CmdRelease)
}

func (a *Actor) finishPlay() {
	if a.end {
		return
	}
	a.playing = false
	a.pushResult()
	a.kind = match.KindSettle
	a.end = true
	a.who = ""
	msg := &match.DoneMsg{RoomID: a.id}
	msg.SetCmd(match.CmdDone)
	if match.Name == "" {
		a.Exit()
		return
	}
	// 用 Send 通知目录，不再等 Reply；目录忘掉这场后场景退出。
	if err := gonet.SendName(match.Name, msg); err != nil {
		slog.Warn("room done", "room", a.id, "err", err)
	}
	a.push(match.KindSettle, "", copySeats(a.seats), a.seats, "")
	a.Exit()
}

func (a *Actor) finishPlayFrom(e gonet.Envelope) {
	a.playing = false
	a.pushResult()
	a.kind = match.KindSettle
	a.end = true
	a.who = ""
	a.callMatch(e, &match.DoneMsg{RoomID: a.id}, match.CmdDone)
}

func (a *Actor) callMatch(e gonet.Envelope, msg gonet.MessageInterface, cmd string) {
	if match.Name == "" {
		a.restoreWho()
		e.Reply(&Reply{Err: "房间目录还没启动"})
		return
	}
	switch m := msg.(type) {
	case *match.DoneMsg:
		m.SetCmd(cmd)
	case *match.ReleaseMsg:
		m.SetCmd(cmd)
	default:
		a.restoreWho()
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	sess, err := a.CallName(match.Name, callTimeout, msg)
	if err != nil {
		a.restoreWho()
		e.Reply(&Reply{Err: err.Error()})
		return
	}
	a.busy = true
	a.pendingEnv = e
	a.sess = sess
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, _ := e.Msg.(*gonet.CallResponse)
	if m == nil || m.Session != a.sess {
		return
	}
	a.busy = false
	if m.Err != nil {
		a.failPending(m.Err.Error(), errors.Is(m.Err, gonet.ErrDead) || errors.Is(m.Err, gonet.ErrUnknownAlias))
		return
	}
	view, _ := m.Value.(*match.View)
	if view != nil && view.Err != "" {
		a.failPending(view.Err, false)
		return
	}
	if a.kind == match.KindSettle {
		a.push(match.KindSettle, a.who, copySeats(a.seats), a.seats, "")
	} else {
		to := append(copySeats(a.seats), a.who)
		a.push(match.KindLeave, a.who, copySeats(a.seats), to, "")
	}
	a.pendingEnv.Reply(&Reply{})
	if a.end {
		a.Exit()
	}
}

func (a *Actor) failPending(text string, exit bool) {
	if a.kind == match.KindLeave {
		a.restoreWho()
	}
	a.pendingEnv.Reply(&Reply{Err: text})
	if exit {
		a.Exit()
	}
}

func (a *Actor) restoreWho() {
	if a.kind != match.KindLeave || a.who == "" || seated(a.seats, a.who) {
		return
	}
	a.seats = append(a.seats, a.who)
}

func (a *Actor) pushBegin() {
	if a.duo == nil {
		return
	}
	msg := &BeginMsg{
		RoomID:  a.id,
		Mode:    a.mode,
		Seed:    a.duo.Seed,
		Seats:   copySeats(a.seats),
		Zones:   a.duo.Zones,
		Players: a.duo.Players(),
		TargetX: a.duo.TargetX,
		TargetY: a.duo.TargetY,
		Index:   a.duo.Index,
		Until:   a.duo.Deadline,
		Speed:   a.duo.Speed,
		FrameHz: FrameHz,
		Targets: TargetCount,
	}
	msg.SetCmd(CmdBegin)
	a.sendAll(msg)
}

func (a *Actor) pushFrame(ops []Op, events []Event) {
	if a.duo == nil {
		return
	}
	msg := &FrameMsg{
		RoomID:  a.id,
		Frame:   a.duo.Frame,
		Ops:     append([]Op(nil), ops...),
		Events:  events,
		Players: a.duo.Players(),
	}
	msg.SetCmd(CmdFrame)
	a.sendAll(msg)
}

func (a *Actor) pushResult() {
	if a.duo == nil {
		return
	}
	msg := &ResultMsg{
		RoomID: a.id,
		Win:    a.duo.Win(),
		Reason: a.duo.Reason(),
		Index:  a.duo.Index,
		Frame:  a.duo.Frame,
	}
	if a.duo.Win() {
		msg.Index = TargetCount
	}
	msg.SetCmd(CmdResult)
	a.sendAll(msg)
}

func (a *Actor) sendAll(msg gonet.MessageInterface) {
	seats := a.seats
	if a.duo != nil {
		seats = make([]string, len(a.duo.players))
		for i := range a.duo.players {
			seats[i] = a.duo.players[i].roleID
		}
	}
	for _, seat := range seats {
		if seat == "" {
			continue
		}
		if err := gonet.SendName(roleAlias(seat), msg); err != nil && !gone(err) {
			slog.Warn("room push", "room", a.id, "roleid", seat, "cmd", msgCmd(msg), "err", err)
		}
	}
}

func (a *Actor) push(kind, who string, seats, to []string, skip string) {
	phase := match.PhasePlay
	if kind == match.KindSettle {
		phase = ""
	}
	msg := &match.NotifyMsg{
		Kind:     kind,
		RoomID:   a.id,
		RoleID:   who,
		Mode:     a.mode,
		Capacity: a.capacity,
		Seats:    seats,
		Phase:    phase,
	}
	msg.SetCmd(match.CmdNotify)
	for _, seat := range to {
		if seat == "" || seat == skip {
			continue
		}
		if err := gonet.SendName(roleAlias(seat), msg); err != nil && !gone(err) {
			slog.Warn("room notify", "room", a.id, "roleid", seat, "kind", kind, "err", err)
		}
	}
}

func roleAlias(roleID string) string {
	if roleID == "" {
		return ""
	}
	return "role/" + roleID
}

func seated(seats []string, roleID string) bool {
	for _, seat := range seats {
		if seat == roleID {
			return true
		}
	}
	return false
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

func randomSeed() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano())
	}
	v := binary.LittleEndian.Uint32(b[:])
	if v == 0 {
		return 1
	}
	return v
}

func msgCmd(msg gonet.MessageInterface) string {
	if c, ok := msg.(interface{ Command() string }); ok {
		return c.Command()
	}
	return ""
}
