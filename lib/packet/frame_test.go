package packet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	data := []byte(`{"roleid":"1"}`)
	if err := Write(&buf, 1, data); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if got := binary.BigEndian.Uint32(raw[:4]); got != uint32(2+len(data)) {
		t.Fatalf("len=%d", got)
	}
	if got := binary.BigEndian.Uint16(raw[4:6]); got != 1 {
		t.Fatalf("cmd=%d", got)
	}
	cmd, got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != 1 || string(got) != string(data) {
		t.Fatalf("cmd=%d data=%s", cmd, got)
	}
}

func TestEmptyData(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, 5, nil); err != nil {
		t.Fatal(err)
	}
	cmd, data, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != 5 || len(data) != 0 {
		t.Fatalf("cmd=%d data=%q", cmd, data)
	}
}

func TestReject(t *testing.T) {
	if err := Write(io.Discard, 0, []byte("x")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("cmd 0: %v", err)
	}
	if err := Write(io.Discard, 1, make([]byte, MaxPayload)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large: %v", err)
	}
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 1)
	buf.Write(hdr[:])
	buf.WriteByte(0)
	if _, _, err := Read(&buf); !errors.Is(err, ErrEmpty) {
		t.Fatalf("short: %v", err)
	}
}
