package harbormgr

import (
	"context"
	"net"
	"testing"
	"time"

	"gonet/services/internal/harborproto"
)

func TestTCPDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	addr, err := Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	writeCmd(t, conn, harborproto.CmdRegisterAddr, &harborproto.RegisterAddr{ID: 1, Addr: "10.0.0.1:1"})
	reg := readBody(t, conn, harborproto.CmdRegisterAddr).(*harborproto.RegisterAddrResult)
	if !reg.OK || !reg.Fresh || reg.ID != 1 || reg.NodeID == 0 || reg.Addr != "10.0.0.1:1" {
		t.Fatalf("register addr: %+v", reg)
	}
	writeCmd(t, conn, harborproto.CmdRegisterName, &harborproto.RegisterName{ID: 2, NodeID: reg.NodeID, Addr: "10.0.0.1:1", Name: "@a"})
	name := readBody(t, conn, harborproto.CmdRegisterName).(*harborproto.RegisterNameResult)
	if !name.OK || name.Name != "@a" {
		t.Fatalf("register name: %+v", name)
	}
	writeCmd(t, conn, harborproto.CmdQueryAddr, &harborproto.QueryAddr{ID: 3, Name: "@a"})
	query := readBody(t, conn, harborproto.CmdQueryAddr).(*harborproto.QueryAddrResult)
	if !query.OK || query.Addr != "10.0.0.1:1" || query.Name != "@a" || query.NodeID != reg.NodeID {
		t.Fatalf("query: %+v", query)
	}
	writeCmd(t, conn, harborproto.CmdHeartbeat, &harborproto.Heartbeat{ID: 4, NodeID: reg.NodeID, Addr: "10.0.0.1:1"})
	beat := readBody(t, conn, harborproto.CmdHeartbeat).(*harborproto.HeartbeatResult)
	if !beat.OK || beat.ID != 4 || beat.NodeID != reg.NodeID {
		t.Fatalf("heartbeat: %+v", beat)
	}

	other, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	writeCmd(t, other, harborproto.CmdRegisterAddr, &harborproto.RegisterAddr{ID: 5, Addr: "10.0.0.1:1"})
	taken := readBody(t, other, harborproto.CmdRegisterAddr).(*harborproto.RegisterAddrResult)
	if taken.OK || taken.Err != harborproto.ErrTaken {
		t.Fatalf("other conn takes addr: %+v", taken)
	}
	writeCmd(t, other, harborproto.CmdHeartbeat, &harborproto.Heartbeat{ID: 6, NodeID: reg.NodeID})
	foreign := readBody(t, other, harborproto.CmdHeartbeat).(*harborproto.HeartbeatResult)
	if foreign.OK || foreign.Err != harborproto.ErrTaken {
		t.Fatalf("other conn heartbeat: %+v", foreign)
	}

	node := reg.NodeID
	conn.Close()
	deadline := time.Now().Add(time.Second)
	var moved *harborproto.RegisterAddrResult
	for {
		writeCmd(t, other, harborproto.CmdRegisterAddr, &harborproto.RegisterAddr{ID: 7, NodeID: node, Addr: "10.0.0.2:1"})
		moved = readBody(t, other, harborproto.CmdRegisterAddr).(*harborproto.RegisterAddrResult)
		if moved.OK && !moved.Fresh && moved.NodeID == node && moved.Addr == "10.0.0.2:1" {
			break
		}
		if moved.Err != harborproto.ErrTaken || time.Now().After(deadline) {
			t.Fatalf("reclaim: %+v", moved)
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeCmd(t, other, harborproto.CmdQueryAddr, &harborproto.QueryAddr{ID: 8, Name: "@a"})
	kept := readBody(t, other, harborproto.CmdQueryAddr).(*harborproto.QueryAddrResult)
	if !kept.OK || kept.Addr != "10.0.0.2:1" || kept.NodeID != node {
		t.Fatalf("kept name: %+v", kept)
	}
}

func TestRestartReplacesDisconnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addr, err := Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	writeCmd(t, conn, harborproto.CmdRegisterAddr, &harborproto.RegisterAddr{ID: 1, Addr: "10.0.0.1:1"})
	reg := readBody(t, conn, harborproto.CmdRegisterAddr).(*harborproto.RegisterAddrResult)
	if !reg.OK || reg.NodeID == 0 {
		t.Fatalf("register: %+v", reg)
	}
	writeCmd(t, conn, harborproto.CmdRegisterName, &harborproto.RegisterName{ID: 2, NodeID: reg.NodeID, Addr: "10.0.0.1:1", Name: "@a"})
	name := readBody(t, conn, harborproto.CmdRegisterName).(*harborproto.RegisterNameResult)
	if !name.OK {
		t.Fatalf("name: %+v", name)
	}
	conn.Close()

	other, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	deadline := time.Now().Add(time.Second)
	var fresh *harborproto.RegisterAddrResult
	for {
		writeCmd(t, other, harborproto.CmdRegisterAddr, &harborproto.RegisterAddr{ID: 3, Addr: "10.0.0.1:1"})
		fresh = readBody(t, other, harborproto.CmdRegisterAddr).(*harborproto.RegisterAddrResult)
		if fresh.OK && fresh.Fresh && fresh.NodeID != 0 && fresh.NodeID != reg.NodeID {
			break
		}
		if fresh.Err != harborproto.ErrTaken || time.Now().After(deadline) {
			t.Fatalf("replace: %+v", fresh)
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeCmd(t, other, harborproto.CmdQueryAddr, &harborproto.QueryAddr{ID: 4, Name: "@a"})
	gone := readBody(t, other, harborproto.CmdQueryAddr).(*harborproto.QueryAddrResult)
	if gone.OK || gone.Err != harborproto.ErrUnknown {
		t.Fatalf("old name: %+v", gone)
	}
}

func writeCmd(t *testing.T, conn net.Conn, cmd uint16, payload any) {
	t.Helper()
	f, err := harborproto.Pack(cmd, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := harborproto.Write(conn, f); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, conn net.Conn, cmd uint16) any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	f, err := harborproto.Read(conn)
	if err != nil {
		t.Fatal(err)
	}
	env, ok := f.Body()
	if !ok {
		t.Fatalf("cmd %d", f.Cmd)
	}
	res, ok := env.(*harborproto.Result)
	if !ok || res.Cmd != cmd {
		t.Fatalf("result %+v", env)
	}
	body, ok := res.Body()
	if !ok {
		t.Fatalf("data cmd %d", res.Cmd)
	}
	return body
}
