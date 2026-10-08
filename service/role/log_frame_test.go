package role

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"protocol/client"
)

func TestLogFrameSkipsHeartbeat(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	logFrame("recv", client.Heartbeat, nil)
	logFrame("send", client.Heartbeat, "ok")
	logFrame("recv", client.SetLevel, map[string]int{"level": 6})

	text := buf.String()
	if strings.Contains(text, "heartbeat") {
		t.Fatalf("heartbeat logged\n%s", text)
	}
	if !strings.Contains(text, "[recv] setlevel") {
		t.Fatalf("missing setlevel\n%s", text)
	}
}
