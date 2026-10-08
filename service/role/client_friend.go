package role

import (
	"game/service/friend"
	"game/store"
	"protocol/client"
)

func (a *Actor) clientErr(cmd, err string) {
	_ = a.write(client.NewErr(cmd, err))
}

func (a *Actor) clientOK(cmd string) {
	_ = a.write(client.NewFriendOK(cmd))
}

func (a *Actor) clientFriendNotify(kind, roleID, name string, tm int64) {
	_ = a.write(client.NewFriendNotify(kind, roleID, name, tm))
}

func (a *Actor) writeFriendList(link uint64, view *friend.View, rows map[string]store.RoleRow, online map[string]bool) {
	if link != a.link || view == nil {
		return
	}
	_ = a.write(client.NewFriendList(
		friendPeople(view.Friends, rows, online),
		friendReqs(view.Incoming, rows, online),
		friendReqs(view.Outgoing, rows, online),
	))
}

func friendPeople(ids []string, rows map[string]store.RoleRow, online map[string]bool) []client.Friend {
	out := make([]client.Friend, 0, len(ids))
	for _, id := range ids {
		out = append(out, friendPerson(id, rows, online))
	}
	return out
}

func friendReqs(reqs []friend.Req, rows map[string]store.RoleRow, online map[string]bool) []client.FriendRequest {
	out := make([]client.FriendRequest, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, client.FriendRequest{
			Friend: friendPerson(req.RoleID, rows, online),
			Time:   req.Time,
		})
	}
	return out
}

func friendPerson(id string, rows map[string]store.RoleRow, online map[string]bool) client.Friend {
	row := rows[id]
	name := row.Name
	if name == "" {
		name = id
	}
	return client.Friend{RoleID: id, Name: name, Level: row.Level, Online: online[id]}
}
