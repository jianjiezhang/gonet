package room

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"time"

	"game/services/internal/actormsg"
	"game/services/internal/names"

	"gonet"
)

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
	if a.id == "" || names.RoomID(a.SelfName()) != a.id {
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
	if err := gonet.RegisterCmd(a, actormsg.CmdRoomSettle, func() gonet.MessageInterface { return &actormsg.RoomSettleMsg{} }, a.onSettle); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdRoomLeave, func() gonet.MessageInterface { return &actormsg.RoomLeaveMsg{} }, a.onLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdRoomOp, func() gonet.MessageInterface { return &actormsg.RoomOpMsg{} }, a.onOp); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdRoomDead, func() gonet.MessageInterface { return &actormsg.RoomDeadMsg{} }, a.onDead); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdRoomTick, func() gonet.MessageInterface { return &actormsg.RoomTickMsg{} }, a.onTick); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdMatchView, func() gonet.MessageInterface { return &actormsg.MatchView{} }, func(gonet.Envelope) {}); err != nil {
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
	msg := &actormsg.RoomTickMsg{}
	msg.SetCmd(actormsg.CmdRoomTick)
	_, err := a.Timeout(frameDur, msg)
	return err
}

func (a *Actor) onOp(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.RoomOpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(roomReply(actormsg.RoomReply{Err: "参数无效"}))
		return
	}
	if !a.playing || a.duo == nil || a.duo.Done() {
		e.Reply(roomReply(actormsg.RoomReply{Err: "还没开打"}))
		return
	}
	seat := a.duo.SeatOf(m.RoleID)
	if seat < 0 {
		e.Reply(roomReply(actormsg.RoomReply{Err: "不在这场"}))
		return
	}
	a.inputs[seat] = sanitizeOp(Op{Ax: m.Ax, Ay: m.Ay, Dash: m.Dash})
	e.Reply(roomReply(actormsg.RoomReply{}))
}

func (a *Actor) onDead(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.RoomDeadMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(roomReply(actormsg.RoomReply{Err: "参数无效"}))
		return
	}
	if !a.playing || a.duo == nil || a.duo.Done() || a.end {
		e.Reply(roomReply(actormsg.RoomReply{Err: "还没开打"}))
		return
	}
	if m.Frame <= 0 || m.Frame > a.duo.Frame {
		e.Reply(roomReply(actormsg.RoomReply{Err: "帧号无效"}))
		return
	}
	if a.duo.SeatOf(m.RoleID) < 0 {
		e.Reply(roomReply(actormsg.RoomReply{Err: "不在这场"}))
		return
	}
	ended := a.duo.Bite(m.RoleID)
	e.Reply(roomReply(actormsg.RoomReply{}))
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
	m, _ := e.Msg.(*actormsg.RoomSettleMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(roomReply(actormsg.RoomReply{Err: "参数无效"}))
		return
	}
	if a.busy {
		e.Reply(roomReply(actormsg.RoomReply{Err: "正在结算"}))
		return
	}
	if !seated(a.seats, m.RoleID) {
		e.Reply(roomReply(actormsg.RoomReply{Err: "不在这场"}))
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
	a.kind = actormsg.KindMatchSettle
	a.end = true
	a.who = m.RoleID
	a.callMatch(e, &actormsg.MatchDoneMsg{RoomID: a.id}, actormsg.CmdMatchDone)
}

func (a *Actor) onLeave(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.RoomLeaveMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(roomReply(actormsg.RoomReply{Err: "参数无效"}))
		return
	}
	if a.busy {
		e.Reply(roomReply(actormsg.RoomReply{Err: "正在结算"}))
		return
	}
	if !seated(a.seats, m.RoleID) {
		e.Reply(roomReply(actormsg.RoomReply{Err: "不在这场"}))
		return
	}
	if a.playing && a.duo != nil && !a.duo.Done() {
		a.duo.MarkDead(m.RoleID)
		a.seats = removeSeat(a.seats, m.RoleID)
		a.who = m.RoleID
		a.kind = actormsg.KindMatchLeave
		if a.duo.Done() {
			a.finishPlayFrom(e)
			return
		}
		a.end = false
		a.callMatch(e, &actormsg.MatchReleaseMsg{RoomID: a.id, RoleID: m.RoleID}, actormsg.CmdMatchRelease)
		return
	}
	a.seats = removeSeat(a.seats, m.RoleID)
	a.who = m.RoleID
	a.kind = actormsg.KindMatchLeave
	if len(a.seats) == 0 {
		a.end = true
		a.callMatch(e, &actormsg.MatchDoneMsg{RoomID: a.id}, actormsg.CmdMatchDone)
		return
	}
	a.end = false
	a.callMatch(e, &actormsg.MatchReleaseMsg{RoomID: a.id, RoleID: m.RoleID}, actormsg.CmdMatchRelease)
}

func (a *Actor) finishPlay() {
	if a.end {
		return
	}
	a.playing = false
	a.pushResult()
	a.kind = actormsg.KindMatchSettle
	a.end = true
	a.who = ""
	msg := &actormsg.MatchDoneMsg{RoomID: a.id}
	msg.SetCmd(actormsg.CmdMatchDone)
	if names.Match == "" {
		a.Exit()
		return
	}
	// 用 Send 通知目录，不再等 actormsg.RoomReply；目录忘掉这场后场景退出。
	if err := gonet.SendName(names.Match, msg); err != nil {
		slog.Warn("room done", "room", a.id, "err", err)
	}
	a.push(actormsg.KindMatchSettle, "", copySeats(a.seats), a.seats, "")
	a.Exit()
}

func (a *Actor) finishPlayFrom(e gonet.Envelope) {
	a.playing = false
	a.pushResult()
	a.kind = actormsg.KindMatchSettle
	a.end = true
	a.who = ""
	a.callMatch(e, &actormsg.MatchDoneMsg{RoomID: a.id}, actormsg.CmdMatchDone)
}

func (a *Actor) callMatch(e gonet.Envelope, msg gonet.MessageInterface, cmd string) {
	if names.Match == "" {
		a.restoreWho()
		e.Reply(roomReply(actormsg.RoomReply{Err: "房间目录还没启动"}))
		return
	}
	switch m := msg.(type) {
	case *actormsg.MatchDoneMsg:
		m.SetCmd(cmd)
	case *actormsg.MatchReleaseMsg:
		m.SetCmd(cmd)
	default:
		a.restoreWho()
		e.Reply(roomReply(actormsg.RoomReply{Err: "参数无效"}))
		return
	}
	sess, err := a.CallName(names.Match, callTimeout, msg)
	if err != nil {
		a.restoreWho()
		e.Reply(roomReply(actormsg.RoomReply{Err: err.Error()}))
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
	view, _ := m.Value.(*actormsg.MatchView)
	if view != nil && view.Err != "" {
		a.failPending(view.Err, false)
		return
	}
	if a.kind == actormsg.KindMatchSettle {
		a.push(actormsg.KindMatchSettle, a.who, copySeats(a.seats), a.seats, "")
	} else {
		to := append(copySeats(a.seats), a.who)
		a.push(actormsg.KindMatchLeave, a.who, copySeats(a.seats), to, "")
	}
	a.pendingEnv.Reply(roomReply(actormsg.RoomReply{}))
	if a.end {
		a.Exit()
	}
}

func (a *Actor) failPending(text string, exit bool) {
	if a.kind == actormsg.KindMatchLeave {
		a.restoreWho()
	}
	a.pendingEnv.Reply(roomReply(actormsg.RoomReply{Err: text}))
	if exit {
		a.Exit()
	}
}

func (a *Actor) restoreWho() {
	if a.kind != actormsg.KindMatchLeave || a.who == "" || seated(a.seats, a.who) {
		return
	}
	a.seats = append(a.seats, a.who)
}

func (a *Actor) pushBegin() {
	if a.duo == nil {
		return
	}
	msg := &actormsg.RoomBeginMsg{
		RoomID:  a.id,
		Mode:    a.mode,
		Seed:    a.duo.Seed,
		Seats:   copySeats(a.seats),
		Zones:   wireZones(a.duo.Zones),
		Players: wirePlayers(a.duo.Players()),
		TargetX: a.duo.TargetX,
		TargetY: a.duo.TargetY,
		Index:   a.duo.Index,
		Until:   a.duo.Deadline,
		Speed:   a.duo.Speed,
		FrameHz: FrameHz,
		Targets: TargetCount,
	}
	msg.SetCmd(actormsg.CmdRoomBegin)
	a.sendAll(msg)
}

func (a *Actor) pushFrame(ops []Op, events []Event) {
	if a.duo == nil {
		return
	}
	msg := &actormsg.RoomFrameMsg{
		RoomID:  a.id,
		Frame:   a.duo.Frame,
		Ops:     wireOps(ops),
		Events:  wireEvents(events),
		Players: wirePlayers(a.duo.Players()),
	}
	msg.SetCmd(actormsg.CmdRoomFrame)
	a.sendAll(msg)
}

func (a *Actor) pushResult() {
	if a.duo == nil {
		return
	}
	msg := &actormsg.RoomResultMsg{
		RoomID: a.id,
		Win:    a.duo.Win(),
		Reason: a.duo.Reason(),
		Index:  a.duo.Index,
		Frame:  a.duo.Frame,
	}
	if a.duo.Win() {
		msg.Index = TargetCount
	}
	msg.SetCmd(actormsg.CmdRoomResult)
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
		if err := gonet.SendName(names.Role(seat), msg); err != nil && !gone(err) {
			slog.Warn("room push", "room", a.id, "roleid", seat, "cmd", msgCmd(msg), "err", err)
		}
	}
}

func (a *Actor) push(kind, who string, seats, to []string, skip string) {
	phase := actormsg.PhaseMatchPlay
	if kind == actormsg.KindMatchSettle {
		phase = ""
	}
	msg := &actormsg.MatchNotifyMsg{
		Kind:     kind,
		RoomID:   a.id,
		RoleID:   who,
		Mode:     a.mode,
		Capacity: a.capacity,
		Seats:    seats,
		Phase:    phase,
	}
	msg.SetCmd(actormsg.CmdMatchNotify)
	for _, seat := range to {
		if seat == "" || seat == skip {
			continue
		}
		if err := gonet.SendName(names.Role(seat), msg); err != nil && !gone(err) {
			slog.Warn("room notify", "room", a.id, "roleid", seat, "kind", kind, "err", err)
		}
	}
}

func wireZones(in []Zone) []actormsg.RoomZone {
	out := make([]actormsg.RoomZone, len(in))
	for i, z := range in {
		out[i] = actormsg.RoomZone{X: z.X, Y: z.Y, R: z.R, Kind: z.Kind}
	}
	return out
}

func wirePlayers(in []PlayerState) []actormsg.RoomPlayerState {
	out := make([]actormsg.RoomPlayerState, len(in))
	for i, p := range in {
		out[i] = actormsg.RoomPlayerState{RoleID: p.RoleID, X: p.X, Y: p.Y, Alive: p.Alive}
	}
	return out
}

func wireOps(in []Op) []actormsg.RoomOp {
	out := make([]actormsg.RoomOp, len(in))
	for i, op := range in {
		out[i] = actormsg.RoomOp{Ax: op.Ax, Ay: op.Ay, Dash: op.Dash}
	}
	return out
}

func wireEvents(in []Event) []actormsg.RoomEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]actormsg.RoomEvent, len(in))
	for i, ev := range in {
		out[i] = actormsg.RoomEvent{
			Kind: ev.Kind, Index: ev.Index, X: ev.X, Y: ev.Y,
			Speed: ev.Speed, Until: ev.Until, Reason: ev.Reason,
		}
	}
	return out
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

func roomReply(v actormsg.RoomReply) *actormsg.RoomReply {
	v.SetCmd(actormsg.CmdRoomReply)
	return &v
}
