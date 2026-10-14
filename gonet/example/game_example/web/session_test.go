package web

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"game/lib/packet"
	"protocol/client"
)

func TestMain(m *testing.M) {
	heartbeatEvery = time.Hour
	os.Exit(m.Run())
}

func TestUpgradeRejectsPlainGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()
	serveSession(rec, req, "127.0.0.1:1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		done <- fakeRole(c)
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSession(w, r, ln.Addr().String())
	}))
	defer srv.Close()

	conn, br := dialWS(t, srv.URL)
	defer conn.Close()

	if err := writeMasked(conn, opBin, mustFrame(t, client.LoginID, []byte(`{"roleid":"42","token":"dev"}`))); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.LoginID, `"ok"`)

	if err := writeMasked(conn, opBin, mustFrame(t, client.HeartbeatID, nil)); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.HeartbeatID, `"ok"`)

	if err := writeMasked(conn, opBin, mustFrame(t, client.RoleInfoID, nil)); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.RoleInfoID, `{"roleid":"42","level":3,"name":"n","gender":0}`)

	if err := writeMasked(conn, opBin, mustFrame(t, client.SetLevelID, []byte(`{"level":4}`))); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.SetLevelID, `{"level":4,"list":[]}`)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake role timeout")
	}
}

func TestSessionWritesHeartbeat(t *testing.T) {
	heartbeatEvery = 20 * time.Millisecond
	t.Cleanup(func() { heartbeatEvery = time.Hour })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- err
			return
		}
		defer c.Close()
		if _, _, err := packet.Read(c); err != nil {
			got <- err
			return
		}
		if err := packet.Write(c, client.LoginID, []byte(`"ok"`)); err != nil {
			got <- err
			return
		}
		id, data, err := packet.Read(c)
		if err != nil {
			got <- err
			return
		}
		if id != client.HeartbeatID || len(data) != 0 {
			got <- io.ErrUnexpectedEOF
			return
		}
		got <- nil
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSession(w, r, ln.Addr().String())
	}))
	defer srv.Close()
	conn, br := dialWS(t, srv.URL)
	defer conn.Close()
	if err := writeMasked(conn, opBin, mustFrame(t, client.LoginID, []byte(`{"roleid":"1","token":"dev"}`))); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.LoginID, `"ok"`)
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat")
	}
}

func TestSessionDropsBadFrame(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- err
			return
		}
		defer c.Close()
		if _, _, err := packet.Read(c); err != nil {
			got <- err
			return
		}
		_ = packet.Write(c, client.LoginID, []byte(`"ok"`))
		_, _, err = packet.Read(c)
		got <- err
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSession(w, r, ln.Addr().String())
	}))
	defer srv.Close()
	conn, br := dialWS(t, srv.URL)
	defer conn.Close()
	if err := writeMasked(conn, opBin, mustFrame(t, client.LoginID, []byte(`{"roleid":"1","token":"dev"}`))); err != nil {
		t.Fatal(err)
	}
	assertFrame(t, br, client.LoginID, `"ok"`)
	if err := writeMasked(conn, opBin, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("bad frame reached the game port")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func fakeRole(c net.Conn) error {
	id, data, err := packet.Read(c)
	if err != nil {
		return err
	}
	if id != client.LoginID || string(data) != `{"roleid":"42","token":"dev"}` {
		return io.ErrUnexpectedEOF
	}
	if err := packet.Write(c, client.LoginID, []byte(`"ok"`)); err != nil {
		return err
	}
	id, data, err = packet.Read(c)
	if err != nil {
		return err
	}
	if id != client.HeartbeatID || len(data) != 0 {
		return io.ErrUnexpectedEOF
	}
	if err := packet.Write(c, client.HeartbeatID, []byte(`"ok"`)); err != nil {
		return err
	}
	id, data, err = packet.Read(c)
	if err != nil {
		return err
	}
	if id != client.RoleInfoID || len(data) != 0 {
		return io.ErrUnexpectedEOF
	}
	if err := packet.Write(c, client.RoleInfoID, []byte(`{"roleid":"42","level":3,"name":"n","gender":0}`)); err != nil {
		return err
	}
	id, data, err = packet.Read(c)
	if err != nil {
		return err
	}
	if id != client.SetLevelID || string(data) != `{"level":4}` {
		return io.ErrUnexpectedEOF
	}
	return packet.Write(c, client.SetLevelID, []byte(`{"level":4,"list":[]}`))
}

func mustFrame(t *testing.T, cmd uint16, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := packet.Write(&buf, cmd, data); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func dialWS(t *testing.T, httpURL string) (net.Conn, *bufio.Reader) {
	t.Helper()
	host := strings.TrimPrefix(httpURL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	req := "GET / HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("handshake %q", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	return conn, br
}

func writeMasked(conn net.Conn, opcode byte, payload []byte) error {
	hdr := []byte{0x80 | opcode}
	n := len(payload)
	if n < 126 {
		hdr = append(hdr, 0x80|byte(n))
	} else {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(n))
		hdr = append(hdr, 0x80|126)
		hdr = append(hdr, b[:]...)
	}
	mask := []byte{1, 2, 3, 4}
	masked := append([]byte(nil), payload...)
	for i := range masked {
		masked[i] ^= mask[i%4]
	}
	if _, err := conn.Write(append(append(hdr, mask...), masked...)); err != nil {
		return err
	}
	return nil
}

func assertFrame(t *testing.T, r *bufio.Reader, cmd uint16, data string) {
	t.Helper()
	op, payload, err := readWS(r, false)
	if err != nil {
		t.Fatal(err)
	}
	if op != opBin {
		t.Fatalf("opcode %d", op)
	}
	gotCmd, got, err := packet.Read(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if gotCmd != cmd || string(got) != data {
		t.Fatalf("frame cmd=%d data=%s", gotCmd, got)
	}
}
