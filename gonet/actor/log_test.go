package actor

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type logProbe struct{ ActorContext }

func (a *logProbe) Init() error {
	slog.Info("hello")
	slog.Warn("careful")
	slog.Error("bad")
	slog.Info("[send] ping")
	return nil
}

func TestInstallLogPrefixDoesNotHang(t *testing.T) {
	old := slog.Default()
	oldW := log.Writer()
	oldFlags := log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(old)
		log.SetOutput(oldW)
		log.SetFlags(oldFlags)
	})
	InstallLogPrefix()
	done := make(chan struct{})
	go func() {
		slog.Info("boot", "k", "v")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("第一条日志卡住了")
	}
}

func TestServiceLogPrefix(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(prefixHandler{next: slog.NewTextHandler(&buf, nil)}))
	t.Cleanup(func() { slog.SetDefault(old) })

	pid, err := SpawnNamed(&logProbe{}, "role/9")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if err := StopActorWait(pid, time.Second); err != nil {
		t.Fatal(err)
	}

	text := buf.String()
	for _, want := range []string{
		"[role/9] hello",
		"[role/9] careful",
		"[role/9] bad",
		"[role/9][send] ping",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s\n%s", want, text)
		}
	}

	stop := EnterService(".watchdog_1")
	slog.Info("[recv] login")
	stop()
	slog.Info("outside")
	text = buf.String()
	if !strings.Contains(text, "[.watchdog_1][recv] login") {
		t.Fatalf("missing recv\n%s", text)
	}
	if strings.Contains(text, "[.watchdog_1] outside") || strings.Contains(text, "[.watchdog_1]outside") {
		t.Fatalf("prefix leaked\n%s", text)
	}
}
