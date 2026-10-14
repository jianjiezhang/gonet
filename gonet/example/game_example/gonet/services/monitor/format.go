package monitor

import (
	"fmt"
	"strings"
)

// Format 把快照打成多行文本，给 debug 口使用。
func Format(s Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "actors=%d timers=%d send_full=%d send_dead=%d call_timeouts=%d slow_dispatch=%d reply_fail=%d\n",
		s.Actors, s.Timers, s.SendFull, s.SendDead, s.CallTimeouts, s.SlowDispatch, s.ReplyFail)
	for _, a := range s.List {
		alias := a.Alias
		if alias == "" {
			alias = "-"
		}
		cmd := a.LastCmd
		if cmd == "" {
			cmd = "-"
		}
		fmt.Fprintf(&b, "pid=%d alias=%s mailbox=%d/%d high=%d status=%d timers=%d pending_calls=%d full=%d dead=%d call_to=%d slow=%d reply_fail=%d last_cmd=%s last_dispatch=%s\n",
			a.PID, alias, a.Mailbox, a.MailboxCap, a.MailboxHigh, a.Status, a.Timers, a.PendingCalls,
			a.SendFull, a.SendDead, a.CallTimeouts, a.SlowDispatch, a.ReplyFail, cmd, a.LastDispatch)
	}
	return b.String()
}
