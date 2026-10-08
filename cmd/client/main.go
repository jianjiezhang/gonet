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
	"syscall"
	"time"

	"game/lib/net"
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

	go readLoop(conn)

	if err := send(conn, client.NewLogin(*roleID, *token)); err != nil {
		slog.Error("login", "err", err)
		os.Exit(1)
	}
	slog.Info("sent login", "roleid", *roleID)

	if err := send(conn, client.NewMissionListReq()); err != nil {
		slog.Error("missionlist", "err", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "commands: missionlist | roleinfo | setlevel <n> | missionfinish <id> | friendlist | friendapply <roleid> | friendagree <roleid> | friendreject <roleid> | frienddelete <roleid> | guildcreate <name> | guildlist | guildapply <guildid> | guildagree <roleid> | guildreject <roleid> | guildkick <roleid> | guildleave | guilddisband")

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
	default:
		slog.Warn("unknown command", "cmd", fields[0])
		return nil
	}
}

func send(conn net.Conn, msg client.Out) error {
	return msg.Send(func(cmd string, body any) error {
		return gamenet.WriteMsg(conn, cmd, body)
	})
}

func readLoop(conn net.Conn) {
	for {
		msg, err := gamenet.ReadMsg(conn)
		if err != nil {
			if err != io.EOF {
				slog.Error("read", "err", err)
			}
			return
		}
		body, _ := client.DecodeRsp(gamenet.Cmd(msg), []byte(gamenet.Data(msg)))
		slog.Info("recv", "cmd", gamenet.Cmd(msg), "body", body)
		if gamenet.Cmd(msg) == client.Kick {
			os.Exit(0)
		}
		if gamenet.Cmd(msg) == client.Login && strings.Contains(gamenet.Data(msg), "auth") {
			slog.Error("login rejected", "data", gamenet.Data(msg))
			os.Exit(1)
		}
	}
}
