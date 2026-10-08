package monitor

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestListenMonitor(t *testing.T) {
	addr, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer StopListen()

	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := c.Write([]byte("monitor\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buf[:n])
	if !strings.Contains(got, "gonet debug") && !strings.Contains(got, "actors=") {
		t.Fatalf("got %q", got)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(got, "actors=") && time.Now().Before(deadline) {
		n, err = c.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		got += string(buf[:n])
	}
	if !strings.Contains(got, "actors=") {
		t.Fatalf("no monitor: %q", got)
	}
	if _, err := c.Write([]byte("quit\n")); err != nil {
		t.Fatal(err)
	}
}

func TestListenTwice(t *testing.T) {
	if _, err := Listen("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer StopListen()
	if _, err := Listen("127.0.0.1:0"); err == nil {
		t.Fatal("expected already listening")
	}
}
