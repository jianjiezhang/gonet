package client

import "testing"

func TestLoginRoundTrip(t *testing.T) {
	msg := NewLogin("39000001", "dev")
	b, err := Pack(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := DecodeReq(msg.Cmd, b)
	if !ok {
		t.Fatal("decode req")
	}
	req := got.(*LoginReq)
	if req.RoleID != "39000001" || req.Token != "dev" {
		t.Fatalf("%+v", req)
	}

	okMsg := NewLoginOK()
	b, err = Pack(okMsg.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := DecodeRsp(okMsg.Cmd, b)
	if !ok || body != AckOK {
		t.Fatalf("%v %v", ok, body)
	}

	fail := NewLoginErr(AuthFail)
	b, err = Pack(fail.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, ok = DecodeRsp(fail.Cmd, b)
	if !ok || body.(*ErrResp).Err != AuthFail {
		t.Fatalf("%v %v", ok, body)
	}
}

func TestMissionFinishBothWays(t *testing.T) {
	req := NewMissionFinish(1001)
	b, err := Pack(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := DecodeReq(req.Cmd, b)
	if !ok || got.(*MissionFinishReq).ID != 1001 {
		t.Fatalf("%v %v", ok, got)
	}

	rsp := NewMissionFinishOK(1001, 1, 2, []Mission{{ID: 1001, Status: 3, Target: 2}})
	b, err = Pack(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := DecodeRsp(rsp.Cmd, b)
	if !ok || body.(*MissionFinishResp).Reward != 1 {
		t.Fatalf("%v %v", ok, body)
	}

	bad := NewErr(MissionFinish, "不能领取")
	b, err = Pack(bad.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, ok = DecodeRsp(bad.Cmd, b)
	if !ok || body.(*ErrResp).Err != "不能领取" {
		t.Fatalf("%v %v", ok, body)
	}
}

func TestOutSend(t *testing.T) {
	var gotCmd string
	var got any
	err := NewKick().Send(func(cmd string, body any) error {
		gotCmd, got = cmd, body
		return nil
	})
	if err != nil || gotCmd != Kick || got != AckDuplicate {
		t.Fatalf("%v %s %v", err, gotCmd, got)
	}
	if err := NewHeartbeat().Send(func(cmd string, body any) error {
		if cmd != Heartbeat || body != nil {
			t.Fatalf("%s %v", cmd, body)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
