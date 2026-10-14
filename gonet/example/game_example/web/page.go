package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"game/lib/net"
	"game/lib/packet"
	"protocol/client"
)

const pageDir = "clientgame/dist"

// Start 在本进程提供登录网页。/api/login 打一帧后关掉。
// /api/session 把浏览器上的同一帧转到玩家口，连接保持到断开。
func Start(pageAddr, gameAddr string) (*http.Server, error) {
	if _, err := os.Stat(pageDir); err != nil {
		return nil, fmt.Errorf("缺少 %s，先在 clientgame 里 npm run build", pageDir)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(w, r, gameAddr)
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		serveSession(w, r, gameAddr)
	})
	mux.Handle("/", http.FileServer(http.Dir(pageDir)))
	srv := &http.Server{Addr: pageAddr, Handler: mux}
	ln, err := net.Listen("tcp", pageAddr)
	if err != nil {
		return nil, err
	}
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("page", "err", err)
		}
	}()
	slog.Info("page listen", "addr", ln.Addr().String())
	return srv, nil
}

// Shutdown 停掉网页。
func Shutdown(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func handleLogin(w http.ResponseWriter, r *http.Request, gameAddr string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		RoleID string `json:"roleid"`
		Token  string `json:"token"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil || json.Unmarshal(body, &req) != nil || req.RoleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "err": "请填写角色号"})
		return
	}
	ok, msg, err := loginOnce(gameAddr, req.RoleID, req.Token)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "err": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "err": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "roleid": req.RoleID})
}

func loginOnce(gameAddr, roleid, token string) (bool, string, error) {
	conn, err := net.DialTimeout("tcp", gameAddr, 3*time.Second)
	if err != nil {
		return false, "", errors.New("连不上游戏服")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if err := gamenet.WriteMsg(conn, client.Login, client.LoginReq{RoleID: roleid, Token: token}); err != nil {
		return false, "", err
	}
	id, data, err := packet.Read(conn)
	if err != nil {
		return false, "", errors.New("连接已断开")
	}
	if id != client.LoginID {
		return false, "登录回复异常", nil
	}
	if string(data) == `"ok"` {
		return true, "", nil
	}
	var body struct {
		Err string `json:"err"`
	}
	if json.Unmarshal(data, &body) == nil && body.Err != "" {
		if body.Err == client.AuthFail {
			return false, "口令不对", nil
		}
		return false, body.Err, nil
	}
	return false, "登录失败", nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
