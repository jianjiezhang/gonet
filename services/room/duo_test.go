package room

import "testing"

func TestDuoPassTen(t *testing.T) {
	d := NewDuo(42, []string{"a", "b"})
	if d.Index != 0 || d.Deadline != TargetFrames {
		t.Fatalf("begin index=%d until=%d", d.Index, d.Deadline)
	}
	for d.Index < TargetCount && !d.Done() {
		ops := []Op{
			{Ax: dirToward(d.players[0].x, d.TargetX), Ay: dirToward(d.players[0].y, d.TargetY)},
			{},
		}
		events := d.Step(ops)
		if d.Frame > TargetFrames*TargetCount+100 {
			t.Fatalf("too many frames %d index=%d", d.Frame, d.Index)
		}
		if len(events) == 0 {
			continue
		}
		last := events[len(events)-1]
		switch last.Kind {
		case EventPass:
			if last.Index != d.Index || last.Until != d.Deadline {
				t.Fatalf("pass %+v want index=%d until=%d", last, d.Index, d.Deadline)
			}
		case EventWin:
			if !d.Win() || d.Reason() != ReasonHit {
				t.Fatalf("win state %+v", d)
			}
		default:
			t.Fatalf("event %+v", last)
		}
	}
	if !d.Done() || !d.Win() {
		t.Fatalf("done=%v win=%v index=%d", d.Done(), d.Win(), d.Index)
	}
	if d.Speed < 5 {
		t.Fatalf("speed %v", d.Speed)
	}
}

func TestDuoTimeout(t *testing.T) {
	d := NewDuo(7, []string{"a", "b"})
	for !d.Done() {
		d.Step([]Op{{}, {}})
		if d.Frame > TargetFrames+5 {
			t.Fatal("not ended")
		}
	}
	if d.Win() || d.Reason() != ReasonTime {
		t.Fatalf("win=%v reason=%s", d.Win(), d.Reason())
	}
}

func TestDuoBiteEndsOnce(t *testing.T) {
	d := NewDuo(1, []string{"a", "b"})
	d.Step([]Op{{Ax: 1}, {}})
	if ended := d.Bite("a"); ended {
		t.Fatal("one death ended the match")
	}
	if d.Players()[0].Alive {
		t.Fatal("a still alive")
	}
	x := d.Players()[0].X
	d.Step([]Op{{Ax: 1}, {Ay: 1}})
	if d.Players()[0].X != x || d.Players()[0].Alive {
		t.Fatalf("dead player moved %+v", d.Players()[0])
	}
	if d.Bite("a") || d.Done() {
		t.Fatal("repeat bite")
	}
	if !d.Bite("b") || !d.Done() || d.Win() || d.Reason() != ReasonBite {
		t.Fatalf("done=%v win=%v reason=%s", d.Done(), d.Win(), d.Reason())
	}
	if d.Bite("b") {
		t.Fatal("bite after end")
	}
}

func TestDuoSameSeedSameTargets(t *testing.T) {
	a := NewDuo(99, []string{"a", "b"})
	b := NewDuo(99, []string{"a", "b"})
	if a.TargetX != b.TargetX || a.TargetY != b.TargetY {
		t.Fatalf("first %+v %+v", a, b)
	}
	for i := 0; i < 3; i++ {
		moveTo(a)
		moveTo(b)
		if a.TargetX != b.TargetX || a.TargetY != b.TargetY || a.Speed != b.Speed {
			t.Fatalf("step %d a=(%v,%v,%v) b=(%v,%v,%v)", i, a.TargetX, a.TargetY, a.Speed, b.TargetX, b.TargetY, b.Speed)
		}
	}
}

func moveTo(d *Duo) {
	for !d.Done() {
		ops := []Op{{Ax: dirToward(d.players[0].x, d.TargetX), Ay: dirToward(d.players[0].y, d.TargetY)}, {}}
		before := d.Index
		events := d.Step(ops)
		for _, ev := range events {
			if ev.Kind == EventPass || ev.Kind == EventWin {
				if d.Index != before && ev.Kind == EventPass {
					return
				}
				if ev.Kind == EventWin {
					return
				}
			}
		}
		if d.Index != before {
			return
		}
	}
}

func dirToward(from, to float64) int {
	if to > from+1 {
		return 1
	}
	if to < from-1 {
		return -1
	}
	return 0
}
