package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"game/lib/net"
	"game/service/room"
	"protocol/client"
)

func init() {
	client.Bind(gamenet.RegisterCmd)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8888", "server address")
	roleID := flag.String("roleid", "39000001", "player role id")
	token := flag.String("token", "dev", "login token")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		slog.Error("dial", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	slog.Info("connected", "addr", *addr, "roleid", *roleID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go readLoop(conn, *roleID)

	if err := send(conn, client.NewLogin(*roleID, *token)); err != nil {
		slog.Error("login", "err", err)
		os.Exit(1)
	}
	slog.Info("sent login", "roleid", *roleID)

	if err := send(conn, client.NewMissionListReq()); err != nil {
		slog.Error("missionlist", "err", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "commands: missionlist | roleinfo | setlevel <n> | missionfinish <id> | friendlist | friendapply <roleid> | friendagree <roleid> | friendreject <roleid> | frienddelete <roleid> | guildcreate <name> | guildlist | guildapply <guildid> | guildagree <roleid> | guildreject <roleid> | guildkick <roleid> | guildleave | guilddisband | roomcreate <mode> <capacity> | roomjoin <roomid> | roomleave | roomstart | roomsettle | roomop <ax> <ay> [dash] | roomdead <frame>")

	lines := make(chan string)
	go readStdin(ctx, lines)

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if err := handleCommand(conn, line); err != nil {
				slog.Error("command", "err", err)
				return
			}
		case <-tick.C:
			if err := send(conn, client.NewHeartbeat()); err != nil {
				slog.Error("heartbeat", "err", err)
				return
			}
		}
	}
}

func readStdin(ctx context.Context, out chan<- string) {
	defer close(out)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		case out <- sc.Text():
		}
	}
}

func handleCommand(conn net.Conn, line string) error {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case client.MissionList:
		slog.Info("sent missionlist")
		return send(conn, client.NewMissionListReq())
	case client.RoleInfo:
		slog.Info("sent roleinfo")
		return send(conn, client.NewRoleInfoReq())
	case client.SetLevel:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "setlevel <n>")
			return nil
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			slog.Error("setlevel 无效", "level", fields[1])
			return nil
		}
		slog.Info("sent setlevel", "level", n)
		return send(conn, client.NewSetLevel(n))
	case client.FriendList:
		slog.Info("sent friendlist")
		return send(conn, client.NewFriendListReq())
	case client.FriendApply, client.FriendAgree, client.FriendReject, client.FriendDelete:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", fields[0]+" <roleid>")
			return nil
		}
		slog.Info("sent "+fields[0], "roleid", fields[1])
		return send(conn, client.NewFriendOp(fields[0], fields[1]))
	case client.MissionFinish:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "missionfinish <id>")
			return nil
		}
		id, err := strconv.Atoi(fields[1])
		if err != nil {
			slog.Error("missionfinish id 无效", "id", fields[1])
			return nil
		}
		slog.Info("sent missionfinish", "id", id)
		return send(conn, client.NewMissionFinish(id))
	case client.GuildCreate:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "guildcreate <name>")
			return nil
		}
		slog.Info("sent guildcreate", "name", fields[1])
		return send(conn, client.NewGuildCreate(fields[1], ""))
	case client.GuildList:
		slog.Info("sent guildlist")
		return send(conn, client.NewGuildListReq())
	case client.GuildApply:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "guildapply <guildid>")
			return nil
		}
		slog.Info("sent guildapply", "guildid", fields[1])
		return send(conn, client.NewGuildApply(fields[1]))
	case client.GuildAgree, client.GuildReject, client.GuildKick:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", fields[0]+" <roleid>")
			return nil
		}
		slog.Info("sent "+fields[0], "roleid", fields[1])
		return send(conn, client.NewGuildTarget(fields[0], fields[1]))
	case client.GuildLeave:
		slog.Info("sent guildleave")
		return send(conn, client.NewGuildLeave())
	case client.GuildDisband:
		slog.Info("sent guilddisband")
		return send(conn, client.NewGuildDisband())
	case client.RoomCreate:
		if len(fields) < 3 {
			slog.Error("usage", "cmd", "roomcreate <mode> <capacity>")
			return nil
		}
		mode, err := strconv.Atoi(fields[1])
		if err != nil {
			slog.Error("roomcreate mode 无效", "mode", fields[1])
			return nil
		}
		capacity, err := strconv.Atoi(fields[2])
		if err != nil {
			slog.Error("roomcreate capacity 无效", "capacity", fields[2])
			return nil
		}
		slog.Info("sent roomcreate", "mode", mode, "capacity", capacity)
		return send(conn, client.NewRoomCreate(mode, capacity))
	case client.RoomJoin:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "roomjoin <roomid>")
			return nil
		}
		slog.Info("sent roomjoin", "roomid", fields[1])
		return send(conn, client.NewRoomJoin(fields[1]))
	case client.RoomLeave:
		slog.Info("sent roomleave")
		return send(conn, client.NewRoomLeave())
	case client.RoomStart:
		slog.Info("sent roomstart")
		return send(conn, client.NewRoomStart())
	case client.RoomSettle:
		slog.Info("sent roomsettle")
		return send(conn, client.NewRoomSettle())
	case client.RoomOp:
		if len(fields) < 3 {
			slog.Error("usage", "cmd", "roomop <ax> <ay> [dash]")
			return nil
		}
		ax, err := strconv.Atoi(fields[1])
		if err != nil {
			slog.Error("roomop ax 无效", "ax", fields[1])
			return nil
		}
		ay, err := strconv.Atoi(fields[2])
		if err != nil {
			slog.Error("roomop ay 无效", "ay", fields[2])
			return nil
		}
		dash := len(fields) >= 4 && (fields[3] == "1" || fields[3] == "dash")
		slog.Info("sent roomop", "ax", ax, "ay", ay, "dash", dash)
		return send(conn, client.NewRoomOp(ax, ay, dash))
	case client.RoomDead:
		if len(fields) < 2 {
			slog.Error("usage", "cmd", "roomdead <frame>")
			return nil
		}
		frame, err := strconv.Atoi(fields[1])
		if err != nil {
			slog.Error("roomdead frame 无效", "frame", fields[1])
			return nil
		}
		slog.Info("sent roomdead", "frame", frame)
		return send(conn, client.NewRoomDead(frame))
	default:
		slog.Warn("unknown command", "cmd", fields[0])
		return nil
	}
}

func send(conn net.Conn, msg client.Out) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return msg.Send(func(cmd string, body any) error {
		return gamenet.WriteMsg(conn, cmd, body)
	})
}

var writeMu sync.Mutex

type localSnakes struct {
	chase    *room.Chase
	speed    float64
	roleID   string
	reported bool
}

func readLoop(conn net.Conn, roleID string) {
	var snakes localSnakes
	snakes.roleID = roleID
	for {
		msg, err := gamenet.ReadMsg(conn)
		if err != nil {
			if err != io.EOF {
				slog.Error("read", "err", err)
			}
			return
		}
		cmd := gamenet.Cmd(msg)
		body, _ := client.DecodeRsp(cmd, []byte(gamenet.Data(msg)))
		slog.Info("recv", "cmd", cmd, "body", body)
		switch cmd {
		case client.Kick:
			os.Exit(0)
		case client.Login:
			if strings.Contains(gamenet.Data(msg), "auth") {
				slog.Error("login rejected", "data", gamenet.Data(msg))
				os.Exit(1)
			}
		case client.RoomBegin:
			if begin, ok := body.(*client.RoomBeginResp); ok {
				snakes.noteBegin(begin)
			}
		case client.RoomFrame:
			if frame, ok := body.(*client.RoomFrameResp); ok {
				snakes.noteFrame(conn, frame)
			}
		case client.RoomResult:
			snakes.chase = nil
		}
	}
}

func (s *localSnakes) noteBegin(begin *client.RoomBeginResp) {
	zones := make([]room.Zone, len(begin.Zones))
	for i, z := range begin.Zones {
		zones[i] = room.Zone{X: z.X, Y: z.Y, R: z.R, Kind: z.Kind}
	}
	s.chase = room.NewChase(begin.Seed, zones)
	s.speed = begin.Speed
	if s.speed < 1 {
		s.speed = 1
	}
	s.reported = false
	slog.Info("snake", "seed", begin.Seed, "heads", headText(s.chase.Heads()))
}

func (s *localSnakes) noteFrame(conn net.Conn, frame *client.RoomFrameResp) {
	if s.chase == nil {
		return
	}
	players := make([]room.PlayerState, len(frame.Players))
	for i, p := range frame.Players {
		players[i] = room.PlayerState{RoleID: p.RoleID, X: p.X, Y: p.Y, Alive: p.Alive}
	}
	dead := s.chase.Step(players, s.speed)
	for _, ev := range frame.Events {
		if (ev.Kind == room.EventPass || ev.Kind == room.EventWin) && ev.Speed >= 1 {
			s.speed = ev.Speed
		}
	}
	if frame.Frame%20 == 0 || len(dead) > 0 {
		slog.Info("snake", "frame", frame.Frame, "heads", headText(s.chase.Heads()), "dead", dead)
	}
	if s.reported {
		return
	}
	for _, id := range dead {
		if id != s.roleID {
			continue
		}
		s.reported = true
		slog.Info("sent roomdead", "frame", frame.Frame)
		if err := send(conn, client.NewRoomDead(frame.Frame)); err != nil {
			slog.Error("roomdead", "err", err)
		}
		return
	}
}

func headText(heads []room.Head) string {
	parts := make([]string, len(heads))
	for i, h := range heads {
		parts[i] = fmt.Sprintf("%.1f,%.1f", h.X, h.Y)
	}
	return strings.Join(parts, ";")
}
