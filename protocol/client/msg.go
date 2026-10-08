package client

import (
	"encoding/json"
)

// 纯文本回复。JSON 是带引号的字符串，不是对象。
const (
	AckOK        = "ok"
	AckDuplicate = "duplicate"
	AuthFail     = "auth"
)

// Out 是业务要写给对端的一条玩家协议。
// 业务调用 New* 得到它，再 Send 给自己的写出函数，不自己拼命令和 map。
type Out struct {
	Cmd  string
	Body any
}

// Send 把这一条交给业务的写出函数。write 为 nil 或命令为空时不写。
func (o Out) Send(write func(cmd string, body any) error) error {
	if write == nil || o.Cmd == "" {
		return nil
	}
	return write(o.Cmd, o.Body)
}

// ErrResp 是失败回复。各命令共用这一份正文。
type ErrResp struct {
	Err string `json:"err"`
}

// LoginReq 是客户端登录正文。
type LoginReq struct {
	RoleID string `json:"roleid"`
	Token  string `json:"token"`
}

// Mission 是一条任务在线上的样子。Reward 是领取时增加的等级，0 表示没有等级奖励。
type Mission struct {
	ID       int `json:"id"`
	Status   int `json:"status"`
	Progress int `json:"progress"`
	Target   int `json:"target"`
	Reward   int `json:"reward"`
}

// MissionListResp 是任务列表回复。
type MissionListResp struct {
	List []Mission `json:"list"`
}

// MissionFinishReq 是领取任务。
type MissionFinishReq struct {
	ID int `json:"id"`
}

// MissionFinishResp 是领取成功。
type MissionFinishResp struct {
	ID     int       `json:"id"`
	Reward int       `json:"reward"`
	Level  int       `json:"level"`
	List   []Mission `json:"list"`
}

// RoleInfoResp 是玩家资料。
type RoleInfoResp struct {
	RoleID string `json:"roleid"`
	Level  int    `json:"level"`
	Name   string `json:"name"`
	Gender int    `json:"gender"`
}

// SetLevelReq 是客户端改等级。
type SetLevelReq struct {
	Level int `json:"level"`
}

// SetLevelResp 是改等级成功。
type SetLevelResp struct {
	Level int       `json:"level"`
	List  []Mission `json:"list"`
}

// FriendOpReq 是好友申请、同意、拒绝、删除的请求。
type FriendOpReq struct {
	RoleID string `json:"roleid"`
}

// Friend 是好友列表里的一个人。
type Friend struct {
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Online bool   `json:"online"`
}

// FriendRequest 是一条申请，比好友多一个时间。
type FriendRequest struct {
	Friend
	Time int64 `json:"time"`
}

// FriendListResp 是好友列表回复。
type FriendListResp struct {
	Friends  []Friend        `json:"friends"`
	Incoming []FriendRequest `json:"incoming"`
	Outgoing []FriendRequest `json:"outgoing"`
}

// FriendNotifyResp 是好友变化推送。
type FriendNotifyResp struct {
	Kind   string `json:"kind"`
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Time   int64  `json:"time"`
}

// GuildCreateReq 是创建公会。
type GuildCreateReq struct {
	Name   string `json:"name"`
	Notice string `json:"notice,omitempty"`
}

// GuildCreateResp 是创建成功。
type GuildCreateResp struct {
	GuildID string `json:"guildid"`
	Name    string `json:"name"`
}

// GuildApplyReq 是申请加入。
type GuildApplyReq struct {
	GuildID string `json:"guildid"`
}

// GuildTargetReq 是同意、拒绝或踢人。
type GuildTargetReq struct {
	RoleID string `json:"roleid"`
}

// GuildMemberCard 是成员列表里的一个人。Rank 1 是会长，2 是成员。
type GuildMemberCard struct {
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Online bool   `json:"online"`
	Rank   int    `json:"rank"`
	Time   int64  `json:"time"`
}

// GuildApplyCard 是一条入会申请。
type GuildApplyCard struct {
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Online bool   `json:"online"`
	Time   int64  `json:"time"`
}

// GuildListResp 是自己所在公会。Applies 只有会长能看到。
type GuildListResp struct {
	GuildID string            `json:"guildid"`
	Name    string            `json:"name"`
	Notice  string            `json:"notice"`
	Leader  string            `json:"leader"`
	Members []GuildMemberCard `json:"members"`
	Applies []GuildApplyCard  `json:"applies"`
}

// GuildNotifyResp 是公会变化推送。
type GuildNotifyResp struct {
	Kind    string `json:"kind"`
	GuildID string `json:"guildid"`
	RoleID  string `json:"roleid"`
	Name    string `json:"name"`
	Time    int64  `json:"time"`
}

func NewText(cmd, text string) Out { return Out{Cmd: cmd, Body: text} }

func NewErr(cmd, err string) Out { return Out{Cmd: cmd, Body: ErrResp{Err: err}} }

func NewLogin(roleID, token string) Out {
	return Out{Cmd: Login, Body: LoginReq{RoleID: roleID, Token: token}}
}

func NewLoginOK() Out { return NewText(Login, AckOK) }

func NewLoginErr(err string) Out { return NewErr(Login, err) }

func NewKick() Out { return NewText(Kick, AckDuplicate) }

func NewPong() Out { return NewText(Pong, AckOK) }

func NewHeartbeat() Out { return Out{Cmd: Heartbeat} }

func NewHeartbeatOK() Out { return NewText(Heartbeat, AckOK) }

func NewMissionListReq() Out { return Out{Cmd: MissionList} }

func NewMissionList(list []Mission) Out {
	return Out{Cmd: MissionList, Body: MissionListResp{List: list}}
}

func NewMissionFinish(id int) Out {
	return Out{Cmd: MissionFinish, Body: MissionFinishReq{ID: id}}
}

func NewMissionFinishOK(id, reward, level int, list []Mission) Out {
	return Out{Cmd: MissionFinish, Body: MissionFinishResp{ID: id, Reward: reward, Level: level, List: list}}
}

func NewSetLevel(level int) Out {
	return Out{Cmd: SetLevel, Body: SetLevelReq{Level: level}}
}

func NewSetLevelOK(level int, list []Mission) Out {
	return Out{Cmd: SetLevel, Body: SetLevelResp{Level: level, List: list}}
}

func NewRoleInfoReq() Out { return Out{Cmd: RoleInfo} }

func NewRoleInfo(roleID, name string, level, gender int) Out {
	return Out{Cmd: RoleInfo, Body: RoleInfoResp{RoleID: roleID, Level: level, Name: name, Gender: gender}}
}

func NewFriendListReq() Out { return Out{Cmd: FriendList} }

func NewFriendList(friends []Friend, incoming, outgoing []FriendRequest) Out {
	return Out{Cmd: FriendList, Body: FriendListResp{Friends: friends, Incoming: incoming, Outgoing: outgoing}}
}

func NewFriendOp(cmd, roleID string) Out {
	return Out{Cmd: cmd, Body: FriendOpReq{RoleID: roleID}}
}

func NewFriendOK(cmd string) Out { return NewText(cmd, AckOK) }

func NewFriendNotify(kind, roleID, name string, tm int64) Out {
	return Out{Cmd: FriendNotify, Body: FriendNotifyResp{Kind: kind, RoleID: roleID, Name: name, Time: tm}}
}

func NewGuildCreate(name, notice string) Out {
	return Out{Cmd: GuildCreate, Body: GuildCreateReq{Name: name, Notice: notice}}
}

func NewGuildCreated(id, name string) Out {
	return Out{Cmd: GuildCreate, Body: GuildCreateResp{GuildID: id, Name: name}}
}

func NewGuildListReq() Out { return Out{Cmd: GuildList} }

func NewGuildList(guildID, name, notice, leader string, members []GuildMemberCard, applies []GuildApplyCard) Out {
	if members == nil {
		members = []GuildMemberCard{}
	}
	if applies == nil {
		applies = []GuildApplyCard{}
	}
	return Out{Cmd: GuildList, Body: GuildListResp{
		GuildID: guildID, Name: name, Notice: notice, Leader: leader, Members: members, Applies: applies,
	}}
}

func NewGuildApply(guildID string) Out {
	return Out{Cmd: GuildApply, Body: GuildApplyReq{GuildID: guildID}}
}

func NewGuildTarget(cmd, roleID string) Out {
	return Out{Cmd: cmd, Body: GuildTargetReq{RoleID: roleID}}
}

func NewGuildLeave() Out   { return Out{Cmd: GuildLeave} }
func NewGuildDisband() Out { return Out{Cmd: GuildDisband} }

func NewGuildNotify(kind, guildID, roleID, name string, tm int64) Out {
	return Out{Cmd: GuildNotify, Body: GuildNotifyResp{Kind: kind, GuildID: guildID, RoleID: roleID, Name: name, Time: tm}}
}

// GuildIDReq 是按公会 id 查询。
type GuildIDReq struct {
	GuildID string `json:"guildid"`
}

// GuildNameReq 是按公会名查询。
type GuildNameReq struct {
	Name string `json:"name"`
}

// GuildBrief 是目录里的一条公会。Members 是人数。会长的名字、等级、在线由 role 补上。
type GuildBrief struct {
	GuildID    string `json:"guildid"`
	Name       string `json:"name"`
	Notice     string `json:"notice"`
	Leader     string `json:"leader"`
	LeaderName string `json:"leadername"`
	Level      int    `json:"level"`
	Online     bool   `json:"online"`
	Members    int    `json:"members"`
}

// GuildsResp 是当前全部公会。
type GuildsResp struct {
	Guilds []GuildBrief `json:"guilds"`
}

func NewGuildsReq() Out { return Out{Cmd: Guilds} }

func NewGuilds(guilds []GuildBrief) Out {
	if guilds == nil {
		guilds = []GuildBrief{}
	}
	return Out{Cmd: Guilds, Body: GuildsResp{Guilds: guilds}}
}

func NewGuildID(id string) Out {
	return Out{Cmd: GuildID, Body: GuildIDReq{GuildID: id}}
}

func NewGuildName(name string) Out {
	return Out{Cmd: GuildName, Body: GuildNameReq{Name: name}}
}

func NewGuildBrief(cmd string, card GuildBrief) Out {
	return Out{Cmd: cmd, Body: card}
}

// Pack 把正文编成 JSON。body 为 nil 时没有正文。
func Pack(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

// DecodeReq 按命令名解开客户端请求。没有正文的命令返回 nil, true。
// 不认识的命令或 JSON 对不上时 ok 为假。
func DecodeReq(cmd string, data []byte) (any, bool) {
	switch cmd {
	case Heartbeat, MissionList, RoleInfo, FriendList, Ping, GuildList, GuildLeave, GuildDisband, Guilds:
		return nil, len(data) == 0 || isEmptyObject(data)
	case Login:
		return unmarshal[LoginReq](data)
	case MissionFinish:
		return unmarshal[MissionFinishReq](data)
	case SetLevel:
		return unmarshal[SetLevelReq](data)
	case FriendApply, FriendAgree, FriendReject, FriendDelete:
		return unmarshal[FriendOpReq](data)
	case GuildCreate:
		return unmarshal[GuildCreateReq](data)
	case GuildApply:
		return unmarshal[GuildApplyReq](data)
	case GuildAgree, GuildReject, GuildKick:
		return unmarshal[GuildTargetReq](data)
	case GuildID:
		return unmarshal[GuildIDReq](data)
	case GuildName:
		return unmarshal[GuildNameReq](data)
	default:
		return nil, false
	}
}

// DecodeRsp 按命令名解开服务端回复。纯文本回复返回 string。
// 带 err 的对象返回 *ErrResp。不认识的命令或 JSON 对不上时 ok 为假。
func DecodeRsp(cmd string, data []byte) (any, bool) {
	if len(data) == 0 {
		return nil, true
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, false
		}
		return s, true
	}
	if hasErr(data) {
		return unmarshal[ErrResp](data)
	}
	switch cmd {
	case MissionList:
		return unmarshal[MissionListResp](data)
	case MissionFinish:
		return unmarshal[MissionFinishResp](data)
	case RoleInfo:
		return unmarshal[RoleInfoResp](data)
	case SetLevel:
		return unmarshal[SetLevelResp](data)
	case FriendList:
		return unmarshal[FriendListResp](data)
	case FriendNotify:
		return unmarshal[FriendNotifyResp](data)
	case GuildCreate:
		return unmarshal[GuildCreateResp](data)
	case GuildList:
		return unmarshal[GuildListResp](data)
	case GuildNotify:
		return unmarshal[GuildNotifyResp](data)
	case Guilds:
		return unmarshal[GuildsResp](data)
	case GuildID, GuildName:
		return unmarshal[GuildBrief](data)
	default:
		return nil, false
	}
}

func unmarshal[T any](data []byte) (any, bool) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, false
	}
	return &v, true
}

func isEmptyObject(data []byte) bool {
	var v map[string]any
	return json.Unmarshal(data, &v) == nil && len(v) == 0
}

func hasErr(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	_, ok := probe["err"]
	return ok
}
