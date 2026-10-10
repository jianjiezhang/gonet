package actor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 括号里是当前 actor 的别名，例如 role/39000001、.watchdog_12。
// 只加在这条协程确实跑在某个 actor 里的时候；mailbox 在 run 里绑定，
// 旁路协程（readLoop、等登录）用 EnterService 带上同一个名字。

type logBind struct {
	pid  uint64
	name string
}

var logBinds sync.Map // goid → logBind

func goid() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	const p = "goroutine "
	if n < len(p)+1 {
		return 0
	}
	var id uint64
	for i := len(p); i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			return id
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

func bindLogPID(pid uint64) func() {
	id := goid()
	logBinds.Store(id, logBind{pid: pid})
	return func() { logBinds.Delete(id) }
}

// EnterService 让当前协程的日志带上 name。退出时调用返回的函数。
func EnterService(name string) func() {
	if name == "" {
		return func() {}
	}
	id := goid()
	logBinds.Store(id, logBind{name: name})
	return func() { logBinds.Delete(id) }
}

func currentService() string {
	v, ok := logBinds.Load(goid())
	if !ok {
		return ""
	}
	b := v.(logBind)
	if b.pid != 0 {
		if name := actorRegistry.aliasOf(b.pid); name != "" {
			return name
		}
	}
	return b.name
}

type prefixHandler struct {
	next  slog.Handler
	attrs []slog.Attr
}

var lineMu sync.Mutex

// InstallLogPrefix 让默认 logger 在 actor 协程里给 info、warn、error 加上服务名。
// 不能包住 slog 自带的 handler：SetDefault 会把 log 的输出指回这个 handler，
// 而自带 handler 又调用 log.Output，第一条日志就会卡死。
func InstallLogPrefix() {
	if _, ok := slog.Default().Handler().(prefixHandler); ok {
		return
	}
	slog.SetDefault(slog.New(prefixHandler{}))
}

func (h prefixHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.next != nil {
		return h.next.Enabled(ctx, level)
	}
	return level >= slog.LevelInfo
}

func (h prefixHandler) Handle(ctx context.Context, r slog.Record) error {
	if name := currentService(); name != "" {
		if strings.HasPrefix(r.Message, "[") {
			r.Message = "[" + name + "]" + r.Message
		} else {
			r.Message = "[" + name + "] " + r.Message
		}
	}
	if h.next != nil {
		return h.next.Handle(ctx, r)
	}
	return writeLine(r, h.attrs)
}

func (h prefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.next != nil {
		h.next = h.next.WithAttrs(attrs)
		return h
	}
	h.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return h
}

func (h prefixHandler) WithGroup(name string) slog.Handler {
	if h.next != nil {
		h.next = h.next.WithGroup(name)
		return h
	}
	return h
}

func writeLine(r slog.Record, pre []slog.Attr) error {
	var b strings.Builder
	tm := r.Time
	if tm.IsZero() {
		tm = time.Now()
	}
	b.WriteString(tm.Format("2006/01/02 15:04:05"))
	b.WriteByte(' ')
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(r.Message)
	writeAttr := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Key == "" {
			return
		}
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(formatValue(a.Value))
	}
	for _, a := range pre {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(a)
		return true
	})
	b.WriteByte('\n')
	lineMu.Lock()
	_, err := os.Stderr.WriteString(b.String())
	lineMu.Unlock()
	return err
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n=\"") {
			return strconv.Quote(s)
		}
		return s
	default:
		s := fmt.Sprint(v.Any())
		if v.Kind() == slog.KindAny && s == "" {
			s = v.String()
		}
		if strings.ContainsAny(s, " \t\n=\"") {
			return strconv.Quote(s)
		}
		return s
	}
}
