package monitor

import (
	"time"

	"gonet/actor"
	"gonet/timer"
)

// Snapshot 是进程内 actor / 定时器的只读快照。
type Snapshot struct {
	Actors       int
	Timers       int
	SendFull     uint64
	SendDead     uint64
	CallTimeouts uint64
	SlowDispatch uint64
	ReplyFail    uint64
	List         []ActorStat
}

// ActorStat 是单个 actor 的观测数据。
type ActorStat struct {
	PID          uint64
	Alias        string
	Mailbox      int
	MailboxCap   int
	MailboxHigh  int
	Status       int
	Timers       int
	PendingCalls int
	SendFull     uint64
	SendDead     uint64
	CallTimeouts uint64
	SlowDispatch uint64
	ReplyFail    uint64
	LastCmd      string
	LastDispatch time.Duration
}

// SnapshotNow 返回当前快照，不向任何 mailbox 投递消息。
func SnapshotNow() Snapshot {
	infos := actor.Infos()
	st := actor.SnapshotStats()
	out := Snapshot{
		Actors:       len(infos),
		Timers:       timer.Count(),
		SendFull:     st.SendFull,
		SendDead:     st.SendDead,
		CallTimeouts: st.CallTimeouts,
		SlowDispatch: st.SlowDispatch,
		ReplyFail:    st.ReplyFail,
		List:         make([]ActorStat, 0, len(infos)),
	}
	for _, a := range infos {
		out.List = append(out.List, ActorStat{
			PID:          a.PID,
			Alias:        a.Alias,
			Mailbox:      a.Mailbox,
			MailboxCap:   a.MailboxCap,
			MailboxHigh:  a.MailboxHigh,
			Status:       a.Status,
			Timers:       timer.CountOf(a.PID),
			PendingCalls: a.PendingCalls,
			SendFull:     a.SendFull,
			SendDead:     a.SendDead,
			CallTimeouts: a.CallTimeouts,
			SlowDispatch: a.SlowDispatch,
			ReplyFail:    a.ReplyFail,
			LastCmd:      a.LastCmd,
			LastDispatch: a.LastDispatch,
		})
	}
	return out
}
