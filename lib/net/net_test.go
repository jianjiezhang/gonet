package gamenet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"game/lib/packet"
)

func TestWriteReadLogin(t *testing.T) {
	if err := RegisterCmd("login", 1); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	payload := map[string]string{"roleid": "1", "token": "dev"}
	if err := WriteMsg(&buf, "login", payload); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), buf.Bytes()...)
	if binary.BigEndian.Uint16(raw[4:6]) != 1 {
		t.Fatalf("cmd=%d", binary.BigEndian.Uint16(raw[4:6]))
	}
	msg, err := ReadMsg(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if Cmd(msg) != "login" || !strings.Contains(Data(msg), `"roleid":"1"`) {
		t.Fatalf("cmd=%s data=%s", Cmd(msg), Data(msg))
	}
}

func TestFrameLog(t *testing.T) {
	if got := FrameLog("send", "heartbeat", nil); got != "[send] heartbeat" {
		t.Fatalf("got %s", got)
	}
	if got := FrameLog("recv", "heartbeat", map[string]string{"cmd": "heartbeat", "data": ""}); got != "[recv] heartbeat" {
		t.Fatalf("got %s", got)
	}
	got := FrameLog("recv", "setlevel", map[string]int{"level": 6})
	if got != `[recv] setlevel {"level":6}` {
		t.Fatalf("got %s", got)
	}
}

func TestRegisterConflict(t *testing.T) {
	if err := RegisterCmd("ping", 3); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCmd("ping", 3); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCmd("ping", 4); err == nil {
		t.Fatal("same name different id")
	}
	if err := RegisterCmd("pong", 3); err == nil {
		t.Fatal("same id different name")
	}
}

func TestUnknownCmd(t *testing.T) {
	if err := WriteMsg(&bytes.Buffer{}, "nope", nil); !errors.Is(err, ErrUnknownCmd) {
		t.Fatalf("write: %v", err)
	}
	var buf bytes.Buffer
	if err := packet.Write(&buf, 99, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	_, err := ReadMsg(&buf)
	if !errors.Is(err, ErrCodec) {
		t.Fatalf("read: %v", err)
	}
}
