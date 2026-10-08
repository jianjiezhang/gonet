package actor

import "time"

// Alive 报告 pid 是否仍在进程表中。
func Alive(pid uint64) bool {
	return actorRegistry.get(pid) != nil
}

// SendFrom 按发送方 from 投递到 to。
func SendFrom(from, to uint64, msg MessageInterface) error {
	return sendFrom(from, to, msg, false)
}

// Info 是单个 actor 给 monitor 用的只读字段。
type Info struct {
	PID          uint64
	Alias        string
	Mailbox      int
	MailboxCap   int
	MailboxHigh  int
	Status       int
	PendingCalls int
	SendFull     uint64
	SendDead     uint64
	CallTimeouts uint64
	SlowDispatch uint64
	ReplyFail    uint64
	LastCmd      string
	LastDispatch time.Duration
}

func Infos() []Info {
	all := actorRegistry.list()
	out := make([]Info, 0, len(all))
	for _, a := range all {
		cmd, _ := a.lastCmd.Load().(string)
		out = append(out, Info{
			PID:          a.pid,
			Alias:        actorRegistry.aliasOf(a.pid),
			Mailbox:      len(a.mailbox),
			MailboxCap:   cap(a.mailbox),
			MailboxHigh:  int(a.mailboxHigh.Load()),
			Status:       int(a.status.Load()),
			PendingCalls: a.pendingCount(),
			SendFull:     a.sendFull.Load(),
			SendDead:     a.sendDead.Load(),
			CallTimeouts: a.callTimeouts.Load(),
			SlowDispatch: a.slowDispatch.Load(),
			ReplyFail:    a.replyFail.Load(),
			LastCmd:      cmd,
			LastDispatch: time.Duration(a.lastDispatch.Load()),
		})
	}
	return out
}
