package room

import "testing"

func TestChaseSameInputSameSnakes(t *testing.T) {
	zones := defaultZones()
	a := NewChase(99, zones)
	b := NewChase(99, zones)
	players := []PlayerState{{RoleID: "p", X: 500, Y: 400, Alive: true}}
	deadA, deadB := -1, -1
	for frame := 1; frame <= 1200; frame++ {
		da := a.Step(players, 1)
		db := b.Step(players, 1)
		ha, hb := a.Heads(), b.Heads()
		for i := range ha {
			if ha[i] != hb[i] {
				t.Fatalf("frame %d head %d %+v %+v", frame, i, ha[i], hb[i])
			}
		}
		ba, bb := a.Bodies(), b.Bodies()
		for i := range ba {
			if len(ba[i]) != len(bb[i]) {
				t.Fatalf("frame %d body len %d %d", frame, len(ba[i]), len(bb[i]))
			}
			for k := range ba[i] {
				if ba[i][k] != bb[i][k] {
					t.Fatalf("frame %d body %d.%d %+v %+v", frame, i, k, ba[i][k], bb[i][k])
				}
			}
		}
		if len(da) != len(db) {
			t.Fatalf("frame %d dead %v %v", frame, da, db)
		}
		if len(da) > 0 && deadA < 0 {
			deadA, deadB = frame, frame
		}
	}
	if deadA < 0 || deadA != deadB {
		t.Fatalf("death %d %d", deadA, deadB)
	}
}

func TestChaseSafeNotChased(t *testing.T) {
	zones := defaultZones()
	var safe Zone
	for _, z := range zones {
		if z.Kind == ZoneSafe {
			safe = z
			break
		}
	}
	c := NewChase(3, zones)
	players := []PlayerState{{RoleID: "p", X: safe.X, Y: safe.Y, Alive: true}}
	for i := 0; i < 80; i++ {
		if dead := c.Step(players, 1); len(dead) != 0 {
			t.Fatalf("safe bite %v", dead)
		}
	}
	for _, h := range c.Heads() {
		if dist(h.X, h.Y, safe.X, safe.Y) <= safe.R {
			t.Fatalf("snake entered safe head=%+v", h)
		}
	}
}

func TestChaseStartIsSafe(t *testing.T) {
	zones := defaultZones()
	var start Zone
	for _, z := range zones {
		if z.Kind == ZoneStart {
			start = z
			break
		}
	}
	c := NewChase(5, zones)
	players := []PlayerState{{RoleID: "p", X: start.X, Y: start.Y, Alive: true}}
	for i := 0; i < 80; i++ {
		if dead := c.Step(players, 1); len(dead) != 0 {
			t.Fatalf("start bite %v", dead)
		}
	}
	for _, h := range c.Heads() {
		if dist(h.X, h.Y, start.X, start.Y) <= start.R {
			t.Fatalf("snake entered start head=%+v", h)
		}
	}
}

func TestChasePassesThroughBody(t *testing.T) {
	c := NewChase(1, defaultZones())
	s := &c.snakes[0]
	s.body = []cell{
		{20, 20}, {19, 20}, {18, 20}, {18, 21}, {19, 21}, {20, 21},
		{21, 21}, {21, 20}, {21, 19}, {20, 19}, {19, 19}, {18, 19},
	}
	s.dirX, s.dirY = 0, 1
	s.straight = 10
	s.aimPrey = true
	s.aimID = 0
	x, y := cellCenter(20, 30)
	players := []PlayerState{{RoleID: "p", X: x, Y: y, Alive: true}}
	c.act(s, players)
	if s.body[0].x != 20 || s.body[0].y != 21 {
		t.Fatalf("did not cross body, head=%+v", s.body[0])
	}
}

func TestChaseBodyFollowsHead(t *testing.T) {
	zones := defaultZones()
	c := NewChase(1, zones)
	players := []PlayerState{{RoleID: "p", X: 900, Y: 400, Alive: true}}
	moved := false
	for i := 0; i < 60; i++ {
		before := c.Bodies()[0]
		c.Step(players, 2)
		after := c.Bodies()[0]
		if len(after) < 2 {
			t.Fatal("short body")
		}
		if after[0] != before[0] {
			moved = true
			if after[1] == before[1] && after[1] == before[0] {
				// first segment should have taken previous head or shifted
			}
			// 第二节应接近原先的头位置（格子蛇：旧头变成第二节）
			if after[1] != before[0] {
				t.Fatalf("body did not follow head: before=%+v after=%+v", before[:3], after[:3])
			}
			break
		}
	}
	if !moved {
		t.Fatal("snake never moved")
	}
}
