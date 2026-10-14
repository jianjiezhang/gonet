package actor

import (
	"sync"

	"gonet/timer"
)

type actorTable struct {
	mu      sync.RWMutex
	actors  map[uint64]*actor
	aliases map[string]uint64 // 一对一：别名 -> pid
	nextPID uint64
}

var actorRegistry actorTable

func init() {
	actorRegistry.actors = make(map[uint64]*actor)
	actorRegistry.aliases = make(map[string]uint64)
}

func (t *actorTable) add(a *actor, name string) error {
	t.mu.Lock()
	if name != "" {
		if _, ok := t.aliases[name]; ok {
			t.mu.Unlock()
			return ErrAliasTaken
		}
	}
	t.nextPID++
	a.pid = t.nextPID
	a.alias = name
	t.actors[a.pid] = a
	if name != "" {
		t.aliases[name] = a.pid
	}
	t.mu.Unlock()
	return nil
}

func (t *actorTable) get(pid uint64) *actor {
	t.mu.RLock()
	a := t.actors[pid]
	t.mu.RUnlock()
	return a
}

func (t *actorTable) getByAlias(name string) *actor {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pid, ok := t.aliases[name]
	if !ok {
		return nil
	}
	return t.actors[pid]
}

func (t *actorTable) rebind(pid uint64, name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.actors[pid]
	if a == nil {
		return ErrDead
	}
	if cur, ok := t.aliases[name]; ok && cur != pid {
		return ErrAliasTaken
	}
	if a.alias != "" && a.alias != name {
		delete(t.aliases, a.alias)
	}
	a.alias = name
	t.aliases[name] = pid
	return nil
}

func (t *actorTable) bind(pid uint64, name string) error {
	t.mu.Lock()
	a := t.actors[pid]
	if a == nil {
		t.mu.Unlock()
		return ErrDead
	}
	if a.alias != "" {
		t.mu.Unlock()
		return ErrAliasBound
	}
	if _, ok := t.aliases[name]; ok {
		t.mu.Unlock()
		return ErrAliasTaken
	}
	a.alias = name
	t.aliases[name] = pid
	t.mu.Unlock()
	return nil
}

func (t *actorTable) unbindName(name string) error {
	t.mu.Lock()
	pid, ok := t.aliases[name]
	if !ok {
		t.mu.Unlock()
		return ErrUnknownAlias
	}
	delete(t.aliases, name)
	if a := t.actors[pid]; a != nil && a.alias == name {
		a.alias = ""
	}
	t.mu.Unlock()
	return nil
}

func (t *actorTable) unbindPID(pid uint64) error {
	t.mu.Lock()
	a := t.actors[pid]
	if a == nil {
		t.mu.Unlock()
		return ErrDead
	}
	if a.alias == "" {
		t.mu.Unlock()
		return ErrUnknownAlias
	}
	delete(t.aliases, a.alias)
	a.alias = ""
	t.mu.Unlock()
	return nil
}

func (t *actorTable) query(name string) (uint64, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pid, ok := t.aliases[name]
	if !ok {
		return 0, ErrUnknownAlias
	}
	return pid, nil
}

func (t *actorTable) aliasOf(pid uint64) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	a := t.actors[pid]
	if a == nil {
		return ""
	}
	return a.alias
}

func (t *actorTable) remove(pid uint64) {
	var name string
	t.mu.Lock()
	if a := t.actors[pid]; a != nil && a.alias != "" {
		name = a.alias
		delete(t.aliases, name)
		a.alias = ""
	}
	delete(t.actors, pid)
	t.mu.Unlock()
	timer.Drop(pid)
	_ = unpublishName(name)
}

func (t *actorTable) list() []*actor {
	t.mu.RLock()
	out := make([]*actor, 0, len(t.actors))
	for _, a := range t.actors {
		out = append(out, a)
	}
	t.mu.RUnlock()
	return out
}
