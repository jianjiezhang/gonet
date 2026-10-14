package role

import (
	"encoding/json"
	"fmt"

	"game/config"
	"game/store"
	"protocol/client"
)

const (
	missionStatusActive = 1
	missionStatusReady  = 2
	missionStatusDone   = 3
)

type missionJSON struct {
	List []missionItem `json:"list"`
}

type missionItem struct {
	ID       int `json:"id"`
	Status   int `json:"status"`
	Progress int `json:"progress"`
}

// Missions 是玩家任务进度。挂在 role.Data 上，同一 mailbox 内直接调。
type Missions struct {
	byID     map[int]*missionItem
	onChange func()
}

func newMissions(data []byte) (*Missions, error) {
	m := &Missions{byID: make(map[int]*missionItem)}
	if len(data) == 0 {
		return m, nil
	}
	var blob missionJSON
	if err := json.Unmarshal(data, &blob); err != nil {
		return nil, fmt.Errorf("mission json: %w", err)
	}
	for i := range blob.List {
		item := blob.List[i]
		if item.ID == 0 {
			continue
		}
		cp := item
		m.byID[cp.ID] = &cp
	}
	return m, nil
}

func (m *Missions) bindChange(fn func()) {
	if m == nil {
		return
	}
	m.onChange = fn
}

func (m *Missions) changed() {
	if m != nil && m.onChange != nil {
		m.onChange()
	}
}

func (m *Missions) get(id int) *missionItem {
	if m == nil {
		return nil
	}
	return m.byID[id]
}

func (m *Missions) put(item missionItem) {
	cp := item
	m.byID[cp.ID] = &cp
}

// Bootstrap 登录后补自动接取，并用当前等级刷新进度。
func (m *Missions) Bootstrap(level int) {
	if m == nil {
		return
	}
	dirty := m.acceptUnlocked()
	if m.applyLevel(level) {
		dirty = true
	}
	if dirty {
		m.changed()
	}
}

func (m *Missions) acceptUnlocked() bool {
	dirty := false
	for _, def := range config.MissionDefs() {
		if m.get(def.ID) != nil {
			continue
		}
		if !m.canAccept(def) {
			continue
		}
		m.put(missionItem{ID: def.ID, Status: missionStatusActive, Progress: 0})
		dirty = true
	}
	return dirty
}

func (m *Missions) canAccept(def config.MissionDef) bool {
	if def.PreID == 0 {
		return true
	}
	pre := m.get(def.PreID)
	return pre != nil && pre.Status == missionStatusDone
}

func (m *Missions) applyLevel(level int) bool {
	dirty := false
	for _, def := range config.MissionDefs() {
		if def.Kind != config.MissionKindLevel {
			continue
		}
		item := m.get(def.ID)
		if item == nil || item.Status == missionStatusDone {
			continue
		}
		progress := level
		status := item.Status
		if progress >= def.Target {
			progress = def.Target
			if status == missionStatusActive {
				status = missionStatusReady
			}
		}
		if progress != item.Progress || status != item.Status {
			m.put(missionItem{ID: item.ID, Status: status, Progress: progress})
			dirty = true
		}
	}
	return dirty
}

// Notify 其它模块推进度。kind 与定义里的 Kind 对应。
func (m *Missions) Notify(kind string, n int) {
	if m == nil || kind != config.MissionKindLevel {
		return
	}
	if m.applyLevel(n) {
		m.changed()
	}
}

func (m *Missions) Claim(id int, level int) (rewardLevel int, err error) {
	if m == nil {
		return 0, fmt.Errorf("mission: nil")
	}
	def, ok := config.LookupMission(id)
	if !ok {
		return 0, fmt.Errorf("mission: 未知任务 %d", id)
	}
	item := m.get(id)
	if item == nil || item.Status != missionStatusReady {
		return 0, fmt.Errorf("mission: 不能领取 %d", id)
	}
	m.put(missionItem{ID: id, Status: missionStatusDone, Progress: def.Target})
	m.acceptUnlocked()
	m.applyLevel(level)
	m.changed()
	return def.RewardLevel, nil
}

func (m *Missions) List() []client.Mission {
	if m == nil {
		return nil
	}
	defs := config.MissionDefs()
	out := make([]client.Mission, 0, len(defs))
	for _, def := range defs {
		item := m.get(def.ID)
		if item == nil {
			continue
		}
		out = append(out, client.Mission{
			ID:       item.ID,
			Status:   item.Status,
			Progress: item.Progress,
			Target:   def.Target,
			Reward:   def.RewardLevel,
		})
	}
	return out
}

func (m *Missions) Blob(roleid string) (store.MissionBlob, error) {
	if m == nil {
		return store.MissionBlob{RoleID: roleid, Data: store.EmptyMissionJSON()}, nil
	}
	blob := missionJSON{List: make([]missionItem, 0, len(m.byID))}
	for _, def := range config.MissionDefs() {
		item := m.get(def.ID)
		if item == nil {
			continue
		}
		blob.List = append(blob.List, *item)
	}
	data, err := json.Marshal(blob)
	if err != nil {
		return store.MissionBlob{}, err
	}
	return store.MissionBlob{RoleID: roleid, Data: data}, nil
}
