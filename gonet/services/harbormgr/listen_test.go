package harbormgr

import (
	"context"
	"net"
	"testing"
	"time"

	"gonet/services/harbormgr/proto"
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

	writeCmd(t, conn, proto.CmdRegisterAddr, &proto.RegisterAddr{ID: 1, Addr: "10.0.0.1:1"})
	reg := readBody(t, conn, proto.CmdRegisterAddr).(*proto.RegisterAddrResult)
	if !reg.OK || !reg.Fresh || reg.ID != 1 || reg.NodeID == 0 || reg.Addr != "10.0.0.1:1" {
		t.Fatalf("register addr: %+v", reg)
	}
	writeCmd(t, conn, proto.CmdRegisterName, &proto.RegisterName{ID: 2, NodeID: reg.NodeID, Addr: "10.0.0.1:1", Name: "@a"})
	name := readBody(t, conn, proto.CmdRegisterName).(*proto.RegisterNameResult)
	if !name.OK || name.Name != "@a" {
		t.Fatalf("register name: %+v", name)
	}
	writeCmd(t, conn, proto.CmdQueryAddr, &proto.QueryAddr{ID: 3, Name: "@a"})
	query := readBody(t, conn, proto.CmdQueryAddr).(*proto.QueryAddrResult)
	if !query.OK || query.Addr != "10.0.0.1:1" || query.Name != "@a" || query.NodeID != reg.NodeID {
		t.Fatalf("query: %+v", query)
	}
	writeCmd(t, conn, proto.CmdHeartbeat, &proto.Heartbeat{ID: 4, NodeID: reg.NodeID, Addr: "10.0.0.1:1"})
	beat := readBody(t, conn, proto.CmdHeartbeat).(*proto.HeartbeatResult)
	if !beat.OK || beat.ID != 4 || beat.NodeID != reg.NodeID {
		t.Fatalf("heartbeat: %+v", beat)
	}

	other, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	writeCmd(t, other, proto.CmdRegisterAddr, &proto.RegisterAddr{ID: 5, Addr: "10.0.0.1:1"})
	taken := readBody(t, other, proto.CmdRegisterAddr).(*proto.RegisterAddrResult)
	if taken.OK || taken.Err != proto.ErrTaken {
		t.Fatalf("other conn takes addr: %+v", taken)
	}
	writeCmd(t, other, proto.CmdHeartbeat, &proto.Heartbeat{ID: 6, NodeID: reg.NodeID})
	foreign := readBody(t, other, proto.CmdHeartbeat).(*proto.HeartbeatResult)
	if foreign.OK || foreign.Err != proto.ErrTaken {
		t.Fatalf("other conn heartbeat: %+v", foreign)
	}

	node := reg.NodeID
	conn.Close()
	deadline := time.Now().Add(time.Second)
	var moved *proto.RegisterAddrResult
	for {
		writeCmd(t, other, proto.CmdRegisterAddr, &proto.RegisterAddr{ID: 7, NodeID: node, Addr: "10.0.0.2:1"})
		moved = readBody(t, other, proto.CmdRegisterAddr).(*proto.RegisterAddrResult)
		if moved.OK && !moved.Fresh && moved.NodeID == node && moved.Addr == "10.0.0.2:1" {
			break
		}
		if moved.Err != proto.ErrTaken || time.Now().After(deadline) {
			t.Fatalf("reclaim: %+v", moved)
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeCmd(t, other, proto.CmdQueryAddr, &proto.QueryAddr{ID: 8, Name: "@a"})
	kept := readBody(t, other, proto.CmdQueryAddr).(*proto.QueryAddrResult)
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
	writeCmd(t, conn, proto.CmdRegisterAddr, &proto.RegisterAddr{ID: 1, Addr: "10.0.0.1:1"})
	reg := readBody(t, conn, proto.CmdRegisterAddr).(*proto.RegisterAddrResult)
	if !reg.OK || reg.NodeID == 0 {
		t.Fatalf("register: %+v", reg)
	}
	writeCmd(t, conn, proto.CmdRegisterName, &proto.RegisterName{ID: 2, NodeID: reg.NodeID, Addr: "10.0.0.1:1", Name: "@a"})
	name := readBody(t, conn, proto.CmdRegisterName).(*proto.RegisterNameResult)
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
	var fresh *proto.RegisterAddrResult
	for {
		writeCmd(t, other, proto.CmdRegisterAddr, &proto.RegisterAddr{ID: 3, Addr: "10.0.0.1:1"})
		fresh = readBody(t, other, proto.CmdRegisterAddr).(*proto.RegisterAddrResult)
		if fresh.OK && fresh.Fresh && fresh.NodeID != 0 && fresh.NodeID != reg.NodeID {
			break
		}
		if fresh.Err != proto.ErrTaken || time.Now().After(deadline) {
			t.Fatalf("replace: %+v", fresh)
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeCmd(t, other, proto.CmdQueryAddr, &proto.QueryAddr{ID: 4, Name: "@a"})
	gone := readBody(t, other, proto.CmdQueryAddr).(*proto.QueryAddrResult)
	if gone.OK || gone.Err != proto.ErrUnknown {
		t.Fatalf("old name: %+v", gone)
	}
}

func writeCmd(t *testing.T, conn net.Conn, cmd uint16, payload any) {
	t.Helper()
	f, err := proto.Pack(cmd, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Write(conn, f); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, conn net.Conn, cmd uint16) any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	f, err := proto.Read(conn)
	if err != nil {
		t.Fatal(err)
	}
	env, ok := f.Body()
	if !ok {
		t.Fatalf("cmd %d", f.Cmd)
	}
	res, ok := env.(*proto.Result)
	if !ok || res.Cmd != cmd {
		t.Fatalf("result %+v", env)
	}
	body, ok := res.Body()
	if !ok {
		t.Fatalf("data cmd %d", res.Cmd)
	}
	return body
}
