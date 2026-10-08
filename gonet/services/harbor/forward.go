package harbor

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr"
	"gonet/services/harbormgr/proto"
)

const maxForward = 64

// peerSend 是 harbor 之间转发的一条 actor 消息。ID 非 0 时对方要回确认。
type peerSend struct {
	ID      uint64 `json:"id,omitempty"`
	Dest    string `json:"dest"`
	From    string `json:"from,omitempty"`
	Cmd     string `json:"cmd"`
	Data    string `json:"data,omitempty"`
	Session uint64 `json:"session,omitempty"`
}

// peerReply 是对方处理完 Call 之后，按调用方别名送回来的 Reply。
type peerReply struct {
	From    string `json:"from"`
	Session uint64 `json:"session"`
	Value   string `json:"value,omitempty"`
	Err     string `json:"err,omitempty"`
}

// peerAck 是对方 harbor 把消息放进 mailbox 之后的确认。
type peerAck struct {
	ID  uint64 `json:"id"`
	Err string `json:"err,omitempty"`
}

type remoteSend struct {
	actor.BaseMessage
	fromName string
	name     string
	cmd      string
	data     string
	done     chan error
	session  uint64
}

type remoteReply struct {
	actor.BaseMessage
	name    string
	session uint64
	value   string
	errText string
}

type pendingReply struct {
	session uint64
	value   string
	errText string
}

type ackWait struct {
	done     chan error
	deadline time.Time
	addr     string
	name     string
}

func publishCluster(name string) error {
	return waitCluster(name, true)
}

func unpublishCluster(name string) error {
	return waitCluster(name, false)
}

func waitCluster(name string, publish bool) error {
	ch := make(chan Result, 1)
	var msg actor.MessageInterface
	if publish {
		m := &RegisterName{Name: name, done: ch}
		m.SetCmd(CmdRegisterName)
		msg = m
	} else {
		m := &UnregisterName{Name: name, done: ch}
		m.SetCmd(CmdUnregisterName)
		msg = m
	}
	if err := actor.SendMemoryName(Name, msg); err != nil {
		return err
	}
	select {
	case r := <-ch:
		if r.OK || (!publish && r.Err == harbormgr.ErrUnknown) {
			return nil
		}
		if r.Err == harbormgr.ErrTaken {
			return actor.ErrAliasTaken
		}
		if r.Err == "" {
			return errors.New("harbor: 别名登记失败")
		}
		return errors.New(r.Err)
	case <-time.After(requestTimeout):
		return errors.New(ErrTimeout)
	}
}

func forwardCluster(fromName, name string, msg *actor.BaseMessage) error {
	if msg == nil {
		return actor.ErrNilMessage
	}
	m := &remoteSend{fromName: fromName, name: name, cmd: msg.Command(), data: msg.Payload()}
	m.SetCmd(cmdRemoteSend)
	return actor.SendMemoryName(Name, m)
}

func forwardClusterAck(fromName, name string, msg *actor.BaseMessage) error {
	if msg == nil {
		return actor.ErrNilMessage
	}
	ch := make(chan error, 1)
	m := &remoteSend{fromName: fromName, name: name, cmd: msg.Command(), data: msg.Payload(), done: ch}
	m.SetCmd(cmdRemoteSend)
	if err := actor.SendMemoryName(Name, m); err != nil {
		return err
	}
	select {
	case err := <-ch:
		return err
	case <-time.After(requestTimeout + time.Second):
		return actor.ErrRemoteTimeout
	}
}

func forwardClusterCall(fromName, name string, msg *actor.BaseMessage, session uint64) error {
	if msg == nil || session == 0 {
		return actor.ErrNilMessage
	}
	if fromName == "" {
		return actor.ErrRemoteCaller
	}
	ch := make(chan error, 1)
	m := &remoteSend{
		fromName: fromName,
		name:     name,
		cmd:      msg.Command(),
		data:     msg.Payload(),
		done:     ch,
		session:  session,
	}
	m.SetCmd(cmdRemoteSend)
	if err := actor.SendMemoryName(Name, m); err != nil {
		return err
	}
	select {
	case err := <-ch:
		return err
	case <-time.After(requestTimeout + time.Second):
		return actor.ErrRemoteTimeout
	}
}

func replyCluster(name string, session uint64, v any, callErr error) {
	raw := ""
	if v != nil && callErr == nil {
		b, err := json.Marshal(v)
		if err != nil {
			slog.Warn("harbor: 跨服回复无法编码", "session", session, "err", err)
			callErr = actor.ErrRemoteDown
		} else {
			raw = string(b)
		}
	}
	m := &remoteReply{name: name, session: session, value: raw, errText: replyCause(callErr)}
	m.SetCmd(cmdRemoteReply)
	if err := actor.SendMemoryName(Name, m); err != nil {
		slog.Warn("harbor: 跨服回复没有送出", "session", session, "err", err)
	}
}

func (a *Actor) onRemoteSend(e actor.Envelope) {
	m, ok := e.Msg.(*remoteSend)
	if !ok {
		return
	}
	if m.name == "" || m.cmd == "" {
		replyDone(m.done, actor.ErrRemoteDown)
		return
	}
	var id uint64
	if m.done != nil {
		id = a.allocID()
		a.acks[id] = &ackWait{done: m.done, deadline: time.Now().Add(requestTimeout), name: m.name}
	}
	if m.session != 0 && m.fromName == "" {
		a.finishAck(id, "unknown")
		return
	}
	body := peerSend{
		ID: id, Dest: m.name, From: m.fromName, Cmd: m.cmd, Data: m.data, Session: m.session,
	}
	if dest, ok := a.remoteName[m.name]; ok {
		if p := a.peers[dest.addr]; p != nil && p.node != 0 && p.node != dest.node {
			a.forgetRemote(m.name)
		} else {
			a.sendRemote(dest, body)
			return
		}
	}
	a.queueForward(body)
}

func (a *Actor) queueForward(body peerSend) {
	if len(a.forwardHold[body.Dest]) >= maxForward {
		slog.Warn("harbor: 跨服发送队列已满", "name", body.Dest)
		a.finishAck(body.ID, "full")
		return
	}
	a.forwardHold[body.Dest] = append(a.forwardHold[body.Dest], body)
	if a.queryInflight(body.Dest) {
		return
	}
	a.sendQuery(body.Dest, asker{})
}

func (a *Actor) queryInflight(name string) bool {
	for _, p := range a.pending {
		if p != nil && p.req == harbormgr.CmdQueryAddr && p.name == name {
			return true
		}
	}
	for _, q := range a.queued {
		if q.name == name {
			return true
		}
	}
	return false
}

func (a *Actor) flushForward(name string) {
	batch := a.forwardHold[name]
	if len(batch) == 0 {
		return
	}
	delete(a.forwardHold, name)
	dest, ok := a.remoteName[name]
	if !ok {
		for _, body := range batch {
			a.finishAck(body.ID, ackCause(a.deliverForward(body)))
		}
		return
	}
	for _, body := range batch {
		a.sendRemote(dest, body)
	}
}

func (a *Actor) dropForward(name, cause string) {
	batch := a.forwardHold[name]
	if len(batch) > 0 {
		slog.Warn("harbor: 跨服消息的目标不存在", "name", name, "n", len(batch))
	}
	delete(a.forwardHold, name)
	for _, body := range batch {
		a.finishAck(body.ID, cause)
	}
}

func (a *Actor) sendRemote(dest remoteDest, body peerSend) {
	if dest.node == a.nodeID || dest.addr == a.addr {
		a.finishAck(body.ID, ackCause(a.deliverForward(body)))
		return
	}
	if w := a.acks[body.ID]; w != nil {
		w.addr = dest.addr
	}
	a.ensurePeer(dest.addr, dest.node)
	if cause := a.enqueuePeer(dest.addr, body); cause != "" {
		a.finishAck(body.ID, cause)
	}
}

func (a *Actor) deliverForward(body peerSend) error {
	msg := &actor.BaseMessage{}
	msg.SetCmd(body.Cmd)
	msg.SetData(body.Data)
	var err error
	if body.Session == 0 {
		err = actor.DeliverLocal(body.Dest, body.From, msg)
	} else {
		err = actor.DeliverCall(body.Dest, body.From, msg, body.Session)
	}
	if err != nil {
		slog.Warn("harbor: 跨服消息无法投递", "name", body.Dest, "err", err)
	}
	return err
}

func (a *Actor) enqueuePeer(addr string, body peerSend) string {
	return a.enqueueCmd(addr, cmdPeerSend, &body)
}

func (a *Actor) enqueueCmd(addr string, cmd uint16, body any) string {
	f, err := proto.Pack(cmd, body)
	if err != nil {
		slog.Warn("harbor: 跨服消息编码失败", "addr", addr, "err", err)
		return "down"
	}
	p := a.peers[addr]
	if p == nil {
		return "down"
	}
	if p.out == nil {
		p.hold = append(p.hold, f)
		return ""
	}
	select {
	case p.out <- f:
		return ""
	default:
		slog.Warn("harbor: 发往节点的队列已满", "addr", addr)
		return "full"
	}
}

func (a *Actor) onPeerReply(m *peerEvent) {
	var body peerReply
	if err := m.frame.Decode(&body); err != nil || body.Session == 0 || body.From == "" {
		return
	}
	pid, _ := actor.Query(body.From)
	actor.FinishCall(pid, body.Session, decodeReply(body.Value), mapReplyErr(body.Err))
}

func (a *Actor) onPeerSend(m *peerEvent, body peerSend) {
	cause := ackCause(a.deliverForward(body))
	if body.ID == 0 {
		return
	}
	a.writeAck(m.out, body.ID, cause)
}

func (a *Actor) onRemoteReply(e actor.Envelope) {
	m, ok := e.Msg.(*remoteReply)
	if !ok || m.session == 0 || m.name == "" {
		return
	}
	if pid, err := actor.Query(m.name); err == nil {
		actor.FinishCall(pid, m.session, decodeReply(m.value), mapReplyErr(m.errText))
		return
	}
	body := pendingReply{session: m.session, value: m.value, errText: m.errText}
	if dest, ok := a.remoteName[m.name]; ok {
		a.sendReply(dest, m.name, body)
		return
	}
	if len(a.replyHold[m.name]) >= maxForward {
		slog.Warn("harbor: 跨服回复队列已满", "name", m.name)
		return
	}
	a.replyHold[m.name] = append(a.replyHold[m.name], body)
	if a.queryInflight(m.name) {
		return
	}
	a.sendQuery(m.name, asker{})
}

func (a *Actor) flushReply(name string) {
	batch := a.replyHold[name]
	if len(batch) == 0 {
		return
	}
	delete(a.replyHold, name)
	if pid, err := actor.Query(name); err == nil {
		for _, body := range batch {
			actor.FinishCall(pid, body.session, decodeReply(body.value), mapReplyErr(body.errText))
		}
		return
	}
	dest, ok := a.remoteName[name]
	if !ok {
		slog.Warn("harbor: 跨服回复找不到调用方", "name", name, "n", len(batch))
		return
	}
	for _, body := range batch {
		a.sendReply(dest, name, body)
	}
}

func (a *Actor) dropReply(name string) {
	batch := a.replyHold[name]
	if len(batch) > 0 {
		slog.Warn("harbor: 跨服回复的调用方不存在", "name", name, "n", len(batch))
	}
	delete(a.replyHold, name)
}

func (a *Actor) sendReply(dest remoteDest, name string, body pendingReply) {
	frame := peerReply{From: name, Session: body.session, Value: body.value, Err: body.errText}
	a.ensurePeer(dest.addr, dest.node)
	if cause := a.enqueueCmd(dest.addr, cmdPeerReply, &frame); cause != "" {
		slog.Warn("harbor: 跨服回复送不出去", "addr", dest.addr, "name", name, "err", cause)
	}
}

func (a *Actor) onPeerAck(m *peerEvent) {
	var ack peerAck
	if err := m.frame.Decode(&ack); err != nil || ack.ID == 0 {
		return
	}
	a.finishAck(ack.ID, ack.Err)
}

func (a *Actor) writeAck(out chan proto.Frame, id uint64, cause string) {
	if out == nil || id == 0 {
		return
	}
	f, err := proto.Pack(cmdPeerAck, &peerAck{ID: id, Err: cause})
	if err != nil {
		return
	}
	select {
	case out <- f:
	default:
	}
}

func (a *Actor) finishAck(id uint64, cause string) {
	if id == 0 {
		return
	}
	w := a.acks[id]
	if w == nil {
		return
	}
	delete(a.acks, id)
	if cause == "unknown" && w.name != "" {
		a.forgetRemote(w.name)
	}
	replyDone(w.done, mapAckErr(cause))
}

func (a *Actor) failAcks(addr, cause string) {
	var ids []uint64
	for id, w := range a.acks {
		if w == nil || addr == "" || w.addr == addr {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		a.finishAck(id, cause)
	}
}

func replyDone(done chan error, err error) {
	if done == nil {
		return
	}
	select {
	case done <- err:
	default:
	}
}

func ackCause(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, actor.ErrUnknownAlias), errors.Is(err, actor.ErrDead):
		return "unknown"
	case errors.Is(err, actor.ErrMailboxFull):
		return "full"
	case errors.Is(err, actor.ErrNotReady):
		return "notready"
	default:
		return "down"
	}
}

func replyCause(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, actor.ErrDead):
		return "dead"
	case errors.Is(err, actor.ErrUnknownAlias):
		return "unknown"
	case errors.Is(err, actor.ErrMailboxFull):
		return "full"
	case errors.Is(err, actor.ErrNotReady):
		return "notready"
	case errors.Is(err, actor.ErrRemoteDown):
		return "down"
	default:
		return err.Error()
	}
}

func mapReplyErr(cause string) error {
	switch cause {
	case "", "unknown", "full", "notready", "timeout", "down":
		return mapAckErr(cause)
	case "dead":
		return actor.ErrDead
	default:
		return errors.New(cause)
	}
}

func decodeReply(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

func mapAckErr(cause string) error {
	switch cause {
	case "":
		return nil
	case "unknown":
		return actor.ErrUnknownAlias
	case "full":
		return actor.ErrMailboxFull
	case "notready":
		return actor.ErrNotReady
	case "timeout":
		return actor.ErrRemoteTimeout
	default:
		return actor.ErrRemoteDown
	}
}
