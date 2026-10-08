package timer

import (
	"errors"
	"sync"
	"time"
)

// ID 是一次性 Timeout 的句柄，进程内递增、不复用。
type ID uint64

var ErrNilFire = errors.New("gonet: timer fire 不能为 nil")

type entry struct {
	id   ID
	pid  uint64
	fire func()
	tm   *time.Timer
}

type hub struct {
	mu    sync.Mutex
	next  ID
	byID  map[ID]*entry
	byPID map[uint64]map[ID]struct{}
}

var timers = hub{
	byID:  make(map[ID]*entry),
	byPID: make(map[uint64]map[ID]struct{}),
}

// Timeout 在 d 后调用 fire。d<=0 表示尽快。实例在本包。
func Timeout(pid uint64, d time.Duration, fire func()) (ID, error) {
	if fire == nil {
		return 0, ErrNilFire
	}
	if d < 0 {
		d = 0
	}

	timers.mu.Lock()
	timers.next++
	id := timers.next
	e := &entry{id: id, pid: pid, fire: fire}
	e.tm = time.AfterFunc(d, func() { run(id) })
	timers.byID[id] = e
	set := timers.byPID[pid]
	if set == nil {
		set = make(map[ID]struct{})
		timers.byPID[pid] = set
	}
	set[id] = struct{}{}
	timers.mu.Unlock()
	return id, nil
}

// Stop 取消尚未触发的定时器。id 无效或已触发则视为成功。
func Stop(id ID) error {
	if id == 0 {
		return nil
	}
	e := timers.take(id)
	if e != nil && e.tm != nil {
		e.tm.Stop()
	}
	return nil
}

func run(id ID) {
	e := timers.take(id)
	if e == nil || e.fire == nil {
		return
	}
	e.fire()
}

func (h *hub) take(id ID) *entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.byID[id]
	if e == nil {
		return nil
	}
	delete(h.byID, id)
	if set := h.byPID[e.pid]; set != nil {
		delete(set, id)
		if len(set) == 0 {
			delete(h.byPID, e.pid)
		}
	}
	return e
}

// Drop 取消该 pid 上所有未触发的定时器。
func Drop(pid uint64) {
	timers.mu.Lock()
	set := timers.byPID[pid]
	delete(timers.byPID, pid)
	var list []*entry
	for id := range set {
		if e := timers.byID[id]; e != nil {
			list = append(list, e)
		}
		delete(timers.byID, id)
	}
	timers.mu.Unlock()
	for _, e := range list {
		if e.tm != nil {
			e.tm.Stop()
		}
	}
}

func Count() int {
	timers.mu.Lock()
	n := len(timers.byID)
	timers.mu.Unlock()
	return n
}

func CountOf(pid uint64) int {
	timers.mu.Lock()
	n := len(timers.byPID[pid])
	timers.mu.Unlock()
	return n
}
