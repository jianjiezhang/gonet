package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"game/config"
	"game/service/friend"
	"game/service/guildmgr"
	"game/service/idgen"
	"game/service/match"
	"game/service/onlinemgr"
	"game/service/watchdog"
	"game/store"
	"game/web"

	"gonet"
	"gonet/redis"
	"gonet/services/harbor"
	"gonet/services/harbormgr"
	"gonet/services/launcher"
)

func main() {
	configPath := flag.String("config", "config/config.toml", "配置文件路径")
	flag.Parse()
	if err := config.Load(*configPath); err != nil {
		slog.Error("load config", "path", *configPath, "err", err)
		os.Exit(1)
	}

	cfg := config.Get()
	slog.Info("server starting", "config", *configPath, "addr", cfg.Addr())

	st, err := store.Open(cfg)
	if err != nil {
		slog.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	slog.Info("store ready", "db", cfg.DBName, "host", cfg.DBHost)

	rd, err := redis.Open(redis.Options{
		Addr:     cfg.RedisAddr(),
		Username: cfg.RedisUser,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err != nil {
		slog.Error("open redis", "err", err)
		os.Exit(1)
	}
	defer rd.Close()

	ip, err := systemIPv4()
	if err != nil {
		slog.Error("local addr", "err", err)
		os.Exit(1)
	}
	stopCluster, nodeID := startCluster(cfg, rd, ip)
	gonet.SetClusterNode(nodeID)
	launcher.Bind(nodeID)
	onlinemgr.Bind(nodeID)
	match.Bind(nodeID)
	watchdog.Bind(nodeID)

	fail := func(msg string, err error) {
		slog.Error(msg, "err", err)
		stopCluster()
		os.Exit(1)
	}
	pid, err := gonet.SpawnNamed(launcher.New(), launcher.Name)
	if err != nil {
		fail("spawn launcher", err)
	}
	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := gonet.WaitInit(initCtx, pid); err != nil {
		initCancel()
		fail("launcher init", err)
	}
	initCancel()

	ping, err := gonet.Pack("ping", struct{}{})
	if err != nil {
		fail("pack ping", err)
	}
	callCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pong, err := gonet.SuspendCallName(callCtx, launcher.Name, ping)
	if err != nil {
		fail("call ping", err)
	}
	slog.Info("got reply", "pong", pong)

	svcCtx, svcCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer svcCancel()
	var idgenPID uint64
	if cfg.Center {
		idgen.UseSeq(redisIDSeq{rd: rd})
		idgenPID, err = launcher.WaitService(svcCtx, idgen.New(), idgen.Name)
		if err != nil {
			fail("new idgen", err)
		}
	}
	onlinePID, err := launcher.WaitService(svcCtx, onlinemgr.New(), onlinemgr.Name)
	if err != nil {
		fail("new onlinemgr", err)
	}
	friendPID, err := launcher.WaitService(svcCtx, friend.New(), friend.Name)
	if err != nil {
		fail("new friend", err)
	}
	guildPID, err := launcher.WaitService(svcCtx, guildmgr.New(), guildmgr.Name)
	if err != nil {
		fail("new guildmgr", err)
	}
	matchPID, err := launcher.WaitService(svcCtx, match.New(), match.Name)
	if err != nil {
		fail("new match", err)
	}

	watchdogPID, err := launcher.WaitService(svcCtx, watchdog.New(), watchdog.Name)
	if err != nil {
		fail("new watchdog", err)
	}
	watchdogPong, err := gonet.SuspendCallName(callCtx, watchdog.Name, ping)
	if err != nil {
		fail("call watchdog ping", err)
	}
	slog.Info("got watchdog reply", "pong", watchdogPong)

	if dbg := cfg.DebugAddr(); dbg != "" {
		bound, err := gonet.ListenDebug(dbg)
		if err != nil {
			fail("listen debug", err)
		}
		defer gonet.StopDebug()
		slog.Info("debug listen", "addr", bound)
	}

	slog.Info("server started", "listen", cfg.Addr())

	var page *http.Server
	if addr := cfg.WebAddr(); addr != "" {
		page, err = web.Start(addr, cfg.GameDialAddr())
		if err != nil {
			fail("page", err)
		}
		defer web.Shutdown(page)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	slog.Info("server shutting down")
	stopCluster()
	if err := gonet.StopActor(watchdogPID); err != nil {
		slog.Error("stop watchdog", "err", err)
	}
	if err := gonet.StopActor(matchPID); err != nil {
		slog.Error("stop match", "err", err)
	}
	if err := gonet.StopActor(friendPID); err != nil {
		slog.Error("stop friend", "err", err)
	}
	if err := gonet.StopActor(guildPID); err != nil {
		slog.Error("stop guildmgr", "err", err)
	}
	if idgenPID != 0 {
		if err := gonet.StopActor(idgenPID); err != nil {
			slog.Error("stop idgen", "err", err)
		}
	}
	if err := gonet.StopActor(onlinePID); err != nil {
		slog.Error("stop onlinemgr", "err", err)
	}
	if err := gonet.StopActor(pid); err != nil {
		slog.Error("stop launcher", "err", err)
	}
	gonet.Stop()
	slog.Info("server stopped")
}

const (
	harbormgrKey     = "harbormgr:addr"
	harbormgrNextKey = "harbormgr:nextnode"
	idgenStateKey    = "idgen:state"
)

// redisNodeSeq 把已经发出的最大 nodeid 放在 Redis，harbormgr 重启后新节点不会领到旧编号。
type redisNodeSeq struct {
	rd *redis.Client
}

func (s redisNodeSeq) Load(ctx context.Context) (uint64, error) {
	v, err := s.rd.Get(ctx, harbormgrNextKey)
	if errors.Is(err, redis.ErrNil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(v, 10, 64)
}

func (s redisNodeSeq) Save(ctx context.Context, nodeID uint64) error {
	return s.rd.Set(ctx, harbormgrNextKey, strconv.FormatUint(nodeID, 10), 0)
}

// redisIDSeq 把每种编号已经发出的最大值放在 Redis。idgen 重启后不会把旧号再发一遍。
type redisIDSeq struct {
	rd *redis.Client
}

func (s redisIDSeq) Load(ctx context.Context) (map[string]uint64, error) {
	v, err := s.rd.Get(ctx, idgenStateKey)
	if errors.Is(err, redis.ErrNil) {
		return map[string]uint64{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]uint64{}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s redisIDSeq) Save(ctx context.Context, counters map[string]uint64) error {
	b, err := json.Marshal(counters)
	if err != nil {
		return err
	}
	return s.rd.Set(ctx, idgenStateKey, string(b), 0)
}

// startCluster 按本机地址和配置端口拉起 harbor。center 节点先启动 harbormgr，并把监听地址写入 Redis。
// 每个节点的 harbor 都从 Redis 读取 harbormgr 地址再连接。返回的函数在进程退出时关掉它们。
func startCluster(cfg config.Config, rd *redis.Client, ip string) (func(), uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mgrListen string
	if cfg.Center {
		harbormgr.UseNodeSeq(redisNodeSeq{rd: rd})
		addr := net.JoinHostPort(ip, strconv.Itoa(cfg.HarborMgrPort))
		bound, err := harbormgr.Start(ctx, addr)
		if err != nil {
			slog.Error("start harbormgr", "err", err)
			os.Exit(1)
		}
		if err := rd.Set(ctx, harbormgrKey, bound, 0); err != nil {
			harbormgr.Stop()
			slog.Error("publish harbormgr", "err", err)
			os.Exit(1)
		}
		mgrListen = bound
		slog.Info("harbormgr started", "addr", bound)
	}
	mgrAddr, err := waitHarbormgr(rd)
	if err != nil {
		dropHarbormgr(rd, mgrListen)
		slog.Error("harbormgr addr", "err", err)
		os.Exit(1)
	}
	harborAddr := net.JoinHostPort(ip, strconv.Itoa(cfg.HarborPort))
	harborCtx, harborCancel := context.WithTimeout(context.Background(), 5*time.Second)
	bound, err := harbor.Start(harborCtx, harborAddr, mgrAddr)
	harborCancel()
	if err != nil {
		dropHarbormgr(rd, mgrListen)
		slog.Error("start harbor", "err", err)
		os.Exit(1)
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	nodeID, err := harbor.WaitReady(readyCtx)
	readyCancel()
	if err != nil {
		harbor.Stop()
		dropHarbormgr(rd, mgrListen)
		slog.Error("harbor node", "err", err)
		os.Exit(1)
	}
	slog.Info("harbor started", "addr", bound, "mgr", mgrAddr, "center", cfg.Center, "node", nodeID, "name", harbor.Name)
	if cfg.Center {
		if err := harbormgr.UseNode(nodeID); err != nil {
			harbor.Stop()
			dropHarbormgr(rd, mgrListen)
			slog.Error("harbormgr alias", "err", err)
			os.Exit(1)
		}
		if err := harbor.Publish(harbormgr.Name); err != nil {
			harbor.Stop()
			dropHarbormgr(rd, mgrListen)
			slog.Error("publish harbormgr", "name", harbormgr.Name, "err", err)
			os.Exit(1)
		}
	}
	return func() {
		harbor.Stop()
		if !cfg.Center {
			return
		}
		dropHarbormgr(rd, mgrListen)
	}, nodeID
}

func dropHarbormgr(rd *redis.Client, mgrListen string) {
	if mgrListen == "" {
		return
	}
	dropCtx, dropCancel := context.WithTimeout(context.Background(), time.Second)
	defer dropCancel()
	cur, err := rd.Get(dropCtx, harbormgrKey)
	if err == nil && cur == mgrListen {
		_, _ = rd.Del(dropCtx, harbormgrKey)
	}
	harbormgr.Stop()
}

func waitHarbormgr(rd *redis.Client) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	var logged bool
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		addr, err := rd.Get(ctx, harbormgrKey)
		cancel()
		if err == nil && addr != "" {
			return addr, nil
		}
		if err != nil && !redis.IsNil(err) {
			return "", err
		}
		if !logged {
			slog.Info("waiting harbormgr addr", "key", harbormgrKey)
			logged = true
		}
		if time.Now().After(deadline) {
			return "", errors.New("等待 harbormgr 地址超时")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// systemIPv4 取一块已启用、非回环网卡上的 IPv4，作为本节点对外地址。
func systemIPv4() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("没有可用的本机 IPv4 地址")
}
