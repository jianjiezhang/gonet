package role

import (
	"testing"

	"game/config"
)

func TestMissionsBootstrapClaim(t *testing.T) {
	m, err := newMissions(nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Bootstrap(1)
	list := m.List()
	if len(list) != 1 || list[0].ID != 1001 || list[0].Status != missionStatusActive || list[0].Reward != 1 {
		t.Fatalf("bootstrap: %+v", list)
	}
	if _, err := m.Claim(1001, 1); err == nil {
		t.Fatal("claim not ready")
	}

	m.Notify(config.MissionKindLevel, 2)
	list = m.List()
	if list[0].Status != missionStatusReady || list[0].Progress != 2 {
		t.Fatalf("ready: %+v", list[0])
	}
	n, err := m.Claim(1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reward=%d", n)
	}
	list = m.List()
	if len(list) != 2 {
		t.Fatalf("unlock: %+v", list)
	}
	if list[0].Status != missionStatusDone || list[1].ID != 1002 || list[1].Status != missionStatusActive {
		t.Fatalf("after claim: %+v", list)
	}

	m.Notify(config.MissionKindLevel, 5)
	if _, err := m.Claim(1002, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Claim(1003, 5); err != nil {
		t.Fatal(err)
	}
	for _, v := range m.List() {
		if v.Status != missionStatusDone {
			t.Fatalf("all done: %+v", m.List())
		}
	}
}
