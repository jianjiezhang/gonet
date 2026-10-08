package role

import (
	"errors"
	"time"

	"game/service/match"
	"game/service/room"
	"protocol/client"

	"gonet"
)

const roomCallTimeout = 3 * time.Second

type roomWait struct {
	link  uint64
	op    string
	scene bool
}

func (a *Actor) onRoomCreate(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*roomCreateMsg)
	mode, capacity := 0, 0
	if m != nil {
		mode, capacity = m.Mode, m.Capacity
	}
	a.callMatch(client.RoomCreate, &match.CreateMsg{
		RoleID: RoleID(a.SelfName()), Mode: mode, Capacity: capacity,
	})
}

func (a *Actor) onRoomJoin(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*roomJoinMsg)
	if m == nil || m.RoomID == "" {
		a.clientErr(client.RoomJoin, "参数无效")
		return
	}
	a.callMatch(client.RoomJoin, &match.JoinMsg{RoleID: RoleID(a.SelfName()), RoomID: m.RoomID})
}

func (a *Actor) onRoomLeave(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	id := RoleID(a.SelfName())
	if a.roomPlay {
		a.callScene(client.RoomLeave, &room.LeaveMsg{RoleID: id})
		return
	}
	a.callMatch(client.RoomLeave, &match.LeaveMsg{RoleID: id})
}

func (a *Actor) onRoomStart(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callMatch(client.RoomStart, &match.StartMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onRoomSettle(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	if !a.roomPlay {
		a.clientErr(client.RoomSettle, "还没开场")
		return
	}
	a.callScene(client.RoomSettle, &room.SettleMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onRoomOp(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	if !a.roomPlay || a.roomID == "" {
		a.clientErr(client.RoomOp, "还没开场")
		return
	}
	m, _ := e.Msg.(*roomOpMsg)
	ax, ay, dash := 0, 0, false
	if m != nil {
		ax, ay, dash = m.Ax, m.Ay, m.Dash
	}
	msg := &room.OpMsg{RoleID: RoleID(a.SelfName()), Ax: ax, Ay: ay, Dash: dash}
	msg.SetCmd(room.CmdOp)
	if err := a.SendName(room.Alias(a.roomID), msg); err != nil {
		if errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead) {
			a.clearRoom()
		}
		a.clientErr(client.RoomOp, err.Error())
	}
}

func (a *Actor) onRoomDead(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	if !a.roomPlay || a.roomID == "" {
		a.clientErr(client.RoomDead, "还没开场")
		return
	}
	m, _ := e.Msg.(*roomDeadMsg)
	frame := 0
	if m != nil {
		frame = m.Frame
	}
	msg := &room.DeadMsg{RoleID: RoleID(a.SelfName()), Frame: frame}
	msg.SetCmd(room.CmdDead)
	if err := a.SendName(room.Alias(a.roomID), msg); err != nil {
		if errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead) {
			a.clearRoom()
		}
		a.clientErr(client.RoomDead, err.Error())
	}
}

func (a *Actor) onRoomBegin(e gonet.Envelope) {
	m, ok := e.Msg.(*room.BeginMsg)
	if !ok || a.conn == nil {
		return
	}
	if m.RoomID != "" {
		a.roomID = m.RoomID
		a.roomPlay = true
	}
	_ = a.write(client.NewRoomBegin(client.RoomBeginResp{
		RoomID: m.RoomID, Mode: m.Mode, Seed: m.Seed, Seats: m.Seats,
		Zones: roomZones(m.Zones), Players: roomPlayers(m.Players),
		TargetX: m.TargetX, TargetY: m.TargetY, Index: m.Index, Until: m.Until,
		Speed: m.Speed, FrameHz: m.FrameHz, Targets: m.Targets,
	}))
}

func (a *Actor) onRoomFrame(e gonet.Envelope) {
	m, ok := e.Msg.(*room.FrameMsg)
	if !ok || a.conn == nil {
		return
	}
	_ = a.write(client.NewRoomFrame(client.RoomFrameResp{
		RoomID: m.RoomID, Frame: m.Frame, Ops: roomOps(m.Ops),
		Events: roomEvents(m.Events), Players: roomPlayers(m.Players),
	}))
}

func (a *Actor) onRoomResult(e gonet.Envelope) {
	m, ok := e.Msg.(*room.ResultMsg)
	if !ok {
		return
	}
	if a.roomID == "" || a.roomID == m.RoomID {
		a.clearRoom()
	}
	if a.conn == nil {
		return
	}
	_ = a.write(client.NewRoomResult(client.RoomResultResp{
		RoomID: m.RoomID, Win: m.Win, Reason: m.Reason, Index: m.Index, Frame: m.Frame,
	}))
}

func (a *Actor) onRoomNotify(e gonet.Envelope) {
	m, ok := e.Msg.(*match.NotifyMsg)
	if !ok {
		return
	}
	self := RoleID(a.SelfName())
	switch m.Kind {
	case match.KindStart:
		if m.RoomID != "" {
			a.roomID = m.RoomID
			a.roomPlay = true
		}
	case match.KindSettle:
		if a.roomID == "" || a.roomID == m.RoomID {
			a.clearRoom()
		}
	case match.KindLeave:
		if m.RoleID == self && (a.roomID == "" || a.roomID == m.RoomID) {
			a.clearRoom()
		}
	}
	if a.conn == nil {
		return
	}
	_ = a.write(client.NewRoomNotify(m.Kind, m.RoomID, m.RoleID, m.Mode, m.Capacity, m.Seats, m.Phase))
}

func (a *Actor) callMatch(op string, msg gonet.MessageInterface) {
	if match.Name == "" {
		a.clientErr(op, "房间服务还没启动")
		return
	}
	switch m := msg.(type) {
	case *match.CreateMsg:
		m.SetCmd(match.CmdCreate)
	case *match.JoinMsg:
		m.SetCmd(match.CmdJoin)
	case *match.LeaveMsg:
		m.SetCmd(match.CmdLeave)
	case *match.StartMsg:
		m.SetCmd(match.CmdStart)
	default:
		a.clientErr(op, "参数无效")
		return
	}
	a.trackRoomCall(op, false, msg, match.Name)
}

func (a *Actor) callScene(op string, msg gonet.MessageInterface) {
	if a.roomID == "" {
		a.clientErr(op, "不在房间")
		return
	}
	switch m := msg.(type) {
	case *room.LeaveMsg:
		m.SetCmd(room.CmdLeave)
	case *room.SettleMsg:
		m.SetCmd(room.CmdSettle)
	default:
		a.clientErr(op, "参数无效")
		return
	}
	a.trackRoomCall(op, true, msg, room.Alias(a.roomID))
}

func (a *Actor) trackRoomCall(op string, scene bool, msg gonet.MessageInterface, name string) {
	if name == "" {
		a.clientErr(op, "房间服务还没启动")
		return
	}
	sess, err := a.CallName(name, roomCallTimeout, msg)
	if err != nil {
		if scene && (errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead)) {
			a.clearRoom()
		}
		a.clientErr(op, err.Error())
		return
	}
	if a.roomWaits == nil {
		a.roomWaits = make(map[uint64]roomWait)
	}
	a.roomWaits[sess] = roomWait{link: a.link, op: op, scene: scene}
}

func (a *Actor) takeRoomResponse(m *gonet.CallResponse) bool {
	w, ok := a.roomWaits[m.Session]
	if !ok {
		return false
	}
	delete(a.roomWaits, m.Session)
	if w.link != a.link {
		return true
	}
	if m.Err != nil {
		if w.scene && (errors.Is(m.Err, gonet.ErrUnknownAlias) || errors.Is(m.Err, gonet.ErrDead)) {
			a.clearRoom()
		}
		a.clientErr(w.op, m.Err.Error())
		return true
	}
	if w.scene {
		view, _ := m.Value.(*room.Reply)
		if view == nil {
			a.clientErr(w.op, "房间没有回复")
			return true
		}
		if view.Err != "" {
			a.clientErr(w.op, view.Err)
			return true
		}
		a.clearRoom()
		a.clientOK(w.op)
		return true
	}
	view, _ := m.Value.(*match.View)
	if view == nil {
		a.clientErr(w.op, "房间没有回复")
		return true
	}
	if view.Err != "" {
		a.clientErr(w.op, view.Err)
		return true
	}
	switch w.op {
	case client.RoomCreate, client.RoomJoin:
		a.roomID = view.RoomID
		a.roomPlay = false
		a.writeRoom(w.op, view)
	case client.RoomStart:
		a.roomID = view.RoomID
		a.roomPlay = view.Phase == match.PhasePlay
		a.writeRoom(w.op, view)
	case client.RoomLeave:
		a.clearRoom()
		a.clientOK(w.op)
	default:
		a.clientErr(w.op, "参数无效")
	}
	return true
}

func (a *Actor) writeRoom(cmd string, view *match.View) {
	if view == nil {
		return
	}
	_ = a.write(client.NewRoomState(cmd, view.RoomID, view.Mode, view.Capacity, view.Seats, view.Phase))
}

func (a *Actor) clearRoom() {
	a.roomID = ""
	a.roomPlay = false
}

func (a *Actor) noteRoomOff() {
	if a.roomID == "" {
		return
	}
	id := RoleID(a.SelfName())
	if a.roomPlay {
		msg := &room.LeaveMsg{RoleID: id}
		msg.SetCmd(room.CmdLeave)
		_ = gonet.SendName(room.Alias(a.roomID), msg)
		return
	}
	if match.Name == "" {
		return
	}
	msg := &match.LeaveMsg{RoleID: id}
	msg.SetCmd(match.CmdLeave)
	_ = gonet.SendName(match.Name, msg)
}

func roomZones(in []room.Zone) []client.RoomZone {
	out := make([]client.RoomZone, len(in))
	for i, z := range in {
		out[i] = client.RoomZone{X: z.X, Y: z.Y, R: z.R, Kind: z.Kind}
	}
	return out
}

func roomPlayers(in []room.PlayerState) []client.RoomPlayer {
	out := make([]client.RoomPlayer, len(in))
	for i, p := range in {
		out[i] = client.RoomPlayer{RoleID: p.RoleID, X: p.X, Y: p.Y, Alive: p.Alive}
	}
	return out
}

func roomOps(in []room.Op) []client.RoomOpFrame {
	out := make([]client.RoomOpFrame, len(in))
	for i, op := range in {
		out[i] = client.RoomOpFrame{Ax: op.Ax, Ay: op.Ay, Dash: op.Dash}
	}
	return out
}

func roomEvents(in []room.Event) []client.RoomEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]client.RoomEvent, len(in))
	for i, ev := range in {
		out[i] = client.RoomEvent{
			Kind: ev.Kind, Index: ev.Index, X: ev.X, Y: ev.Y,
			Speed: ev.Speed, Until: ev.Until, Reason: ev.Reason,
		}
	}
	return out
}
