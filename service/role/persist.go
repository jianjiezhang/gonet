package role

import (
	"log/slog"
	"time"

	"gonet"
)

const persistInterval = 30 * time.Second
const persistTimeout = 2 * time.Second

func (a *Actor) markDirty() {
	a.dirty = true
}

func (a *Actor) startPersist() error {
	return a.armPersist()
}

func (a *Actor) onPersist(e gonet.Envelope) {
	a.flushSave(true)
	a.armPersist()
	e.Reply("ok")
}

func (a *Actor) armPersist() error {
	msg := &PersistMsg{}
	msg.SetCmd(cmdPersist)
	_, err := gonet.Timeout(a.Self(), persistInterval, msg)
	return err
}

func (a *Actor) flushSave(async bool) {
	row, blob, err := a.snapshot()
	if err != nil {
		return
	}
	if async {
		if !a.dirty {
			return
		}
		a.dirty = false
		a.saveWG.Add(1)
		go func() {
			defer a.saveWG.Done()
			if err := SaveRole(row); err != nil {
				slog.Error("persist async", "roleid", row.RoleID, "err", err)
			}
			if err := SaveMission(blob); err != nil {
				slog.Error("persist async mission", "roleid", blob.RoleID, "err", err)
			}
		}()
		return
	}
	if err := saveRoleTimeout(persistTimeout, row); err != nil {
		slog.Error("persist term", "roleid", row.RoleID, "err", err)
	}
	if err := saveMissionTimeout(persistTimeout, blob); err != nil {
		slog.Error("persist term mission", "roleid", blob.RoleID, "err", err)
	}
	a.dirty = false
}
