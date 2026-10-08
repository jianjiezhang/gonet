package actor

import (
	"log/slog"
	"time"

	"gonet/timer"
)

func scheduleTimeout(pid uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	if msg == nil {
		return 0, ErrNilMessage
	}
	if actorRegistry.get(pid) == nil {
		return 0, ErrDead
	}
	id, err := timer.Timeout(pid, d, func() {
		err := sendFrom(pid, pid, msg, false)
		if err != nil && err != ErrDead {
			slog.Error("gonet: 定时器投递失败", "pid", pid, "err", err)
		}
	})
	if err != nil {
		return 0, err
	}
	if actorRegistry.get(pid) == nil {
		_ = timer.Stop(id)
		return 0, ErrDead
	}
	return uint64(id), nil
}

func stopScheduled(id uint64) {
	if id == 0 {
		return
	}
	_ = timer.Stop(timer.ID(id))
}

// Timeout 到期后 Send 给 pid。只 Send，不 Call。
func Timeout(pid uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return scheduleTimeout(pid, d, msg)
}
