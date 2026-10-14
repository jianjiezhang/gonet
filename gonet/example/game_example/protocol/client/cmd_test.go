package client

import "testing"

func TestBindUnique(t *testing.T) {
	names := map[string]uint16{}
	ids := map[uint16]string{}
	Bind(func(name string, id uint16) error {
		if name == "" || id == 0 {
			t.Fatalf("empty pair %q %d", name, id)
		}
		if _, ok := names[name]; ok {
			t.Fatalf("duplicate name %s", name)
		}
		if _, ok := ids[id]; ok {
			t.Fatalf("duplicate id %d", id)
		}
		names[name] = id
		ids[id] = name
		return nil
	})
	if names[Login] != LoginID || ids[FriendNotifyID] != FriendNotify {
		t.Fatalf("login=%d notify=%s", names[Login], ids[FriendNotifyID])
	}
}
