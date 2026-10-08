package room

import (
	"math"
)

const (
	// ModeDuo 是两人同场信使。其它模式仍只是空房间。
	ModeDuo = 4

	FrameHz       = 20
	FrameDT       = 0.05
	TargetCount   = 10
	TargetSeconds = 30
	TargetFrames  = TargetSeconds * FrameHz
	TargetRadius  = 28.0
	SpeedStep     = 1.2

	WorldW = 56 * 32
	WorldH = 36 * 32
	Cell   = 32

	PlayerR     = 11.0
	PlayerSpeed = 272.0
	DashTime    = 0.14
	DashCD      = 1.45
	DashMult    = 2.0
	StartRadius = 160.0
	SafeRadius  = 52.0

	ZoneSafe  = "safe"
	ZoneStart = "start"

	EventPass    = "pass"
	EventFail    = "fail"
	EventWin     = "win"
	ReasonHit    = "hit"
	ReasonTime   = "timeout"
	ReasonEmpty  = "empty"
	ReasonManual = "manual"
	ReasonBite   = "bite"

	SnakeCount = 2
	SnakeBase  = 140.0
	SnakeR     = 16.0
	KillRadius = 25.0
)

// Op 是一个座位这一帧的操作。Ax/Ay 只能是 -1、0、1。
type Op struct {
	Ax   int  `json:"ax"`
	Ay   int  `json:"ay"`
	Dash bool `json:"dash"`
}

// Zone 是场上固定区域。
type Zone struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	R    float64 `json:"r"`
	Kind string  `json:"kind"`
}

// Event 是这一帧服务器判定出来的事。
type Event struct {
	Kind   string  `json:"kind"`
	Index  int     `json:"index,omitempty"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Speed  float64 `json:"speed,omitempty"`
	Until  int     `json:"until,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// PlayerState 是服务器推的玩家位置。
type PlayerState struct {
	RoleID string  `json:"roleid"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Alive  bool    `json:"alive"`
}

type player struct {
	roleID string
	x, y   float64
	alive  bool
	dashT  float64
	dashCD float64
	last   Op
}

// Duo 是模式 4 的服务器侧演算：只推两个玩家，判目标。不算蛇。
type Duo struct {
	Seed     uint32
	Frame    int
	Index    int
	TargetX  float64
	TargetY  float64
	Deadline int
	Speed    float64
	Zones    []Zone
	players  []player
	rng      uint32
	done     bool
	win      bool
	reason   string
}

func cellCenter(cx, cy int) (float64, float64) {
	return float64(cx*Cell + Cell/2), float64(cy*Cell + Cell/2)
}

func defaultZones() []Zone {
	// 底部大出发安全区 + 场上两个小安全区。没有单独的庇护区。
	sx, sy := cellCenter(14, 10)
	hx, hy := cellCenter(42, 14)
	return []Zone{
		{X: sx, Y: sy, R: SafeRadius, Kind: ZoneSafe},
		{X: hx, Y: hy, R: SafeRadius, Kind: ZoneSafe},
		{X: float64(WorldW) / 2, Y: float64(WorldH) - 72, R: StartRadius, Kind: ZoneStart},
	}
}

func startSpots(n int) []struct{ x, y float64 } {
	cx := float64(WorldW) / 2
	cy := float64(WorldH) - 72
	switch n {
	case 1:
		return []struct{ x, y float64 }{{cx, cy}}
	default:
		return []struct{ x, y float64 }{
			{cx - 40, cy},
			{cx + 40, cy},
		}
	}
}

// NewDuo 用种子和座位表开一局。座位顺序就是座位号。
func NewDuo(seed uint32, seats []string) *Duo {
	if seed == 0 {
		seed = 1
	}
	d := &Duo{
		Seed:  seed,
		Speed: 1,
		Zones: defaultZones(),
		rng:   seed ^ 0x9e3779b9,
	}
	spots := startSpots(len(seats))
	d.players = make([]player, len(seats))
	for i, id := range seats {
		spot := spots[i%len(spots)]
		d.players[i] = player{
			roleID: id,
			x:      spot.x,
			y:      spot.y,
			alive:  true,
			last:   Op{},
		}
	}
	d.placeTarget()
	return d
}

func (d *Duo) SeatOf(roleID string) int {
	for i := range d.players {
		if d.players[i].roleID == roleID {
			return i
		}
	}
	return -1
}

func (d *Duo) Players() []PlayerState {
	out := make([]PlayerState, len(d.players))
	for i := range d.players {
		p := &d.players[i]
		out[i] = PlayerState{RoleID: p.roleID, X: p.x, Y: p.y, Alive: p.alive}
	}
	return out
}

func (d *Duo) Done() bool { return d.done }
func (d *Duo) Win() bool  { return d.win }
func (d *Duo) Reason() string {
	return d.reason
}

// Step 推进一帧。ops 按座位；缺的座位沿用上一帧。返回这一帧事件。
func (d *Duo) Step(ops []Op) []Event {
	if d.done {
		return nil
	}
	d.Frame++
	for i := range d.players {
		op := d.players[i].last
		if i < len(ops) {
			op = sanitizeOp(ops[i])
			d.players[i].last = op
		}
		d.stepPlayer(i, op)
	}
	var events []Event
	if hit := d.anyoneHit(); hit >= 0 {
		events = append(events, d.onPass()...)
	} else if d.Frame >= d.Deadline {
		d.finish(false, ReasonTime)
		events = append(events, Event{Kind: EventFail, Reason: ReasonTime, Index: d.Index + 1})
	}
	return events
}

func (d *Duo) ForceFail(reason string) []Event {
	if d.done {
		return nil
	}
	if reason == "" {
		reason = ReasonManual
	}
	d.finish(false, reason)
	return []Event{{Kind: EventFail, Reason: reason, Index: d.Index + 1}}
}

func (d *Duo) MarkDead(roleID string) {
	d.markOut(roleID, ReasonEmpty)
}

// Bite 记下这个座位被蛇咬到。返回值表示这场因此结束。
func (d *Duo) Bite(roleID string) bool {
	return d.markOut(roleID, ReasonBite)
}

func (d *Duo) markOut(roleID, reason string) bool {
	i := d.SeatOf(roleID)
	if i < 0 || d.done || !d.players[i].alive {
		return false
	}
	d.players[i].alive = false
	if d.aliveCount() == 0 {
		d.finish(false, reason)
		return true
	}
	return false
}

func (d *Duo) aliveCount() int {
	n := 0
	for i := range d.players {
		if d.players[i].alive {
			n++
		}
	}
	return n
}

func (d *Duo) anyoneHit() int {
	for i := range d.players {
		p := &d.players[i]
		if !p.alive {
			continue
		}
		if dist(p.x, p.y, d.TargetX, d.TargetY) <= TargetRadius+PlayerR {
			return i
		}
	}
	return -1
}

func (d *Duo) onPass() []Event {
	passed := d.Index + 1
	if passed >= TargetCount {
		d.finish(true, ReasonHit)
		return []Event{{
			Kind:   EventWin,
			Index:  passed,
			X:      d.TargetX,
			Y:      d.TargetY,
			Speed:  d.Speed,
			Reason: ReasonHit,
		}}
	}
	d.Index = passed
	d.Speed *= SpeedStep
	d.placeTarget()
	return []Event{{
		Kind:  EventPass,
		Index: d.Index,
		X:     d.TargetX,
		Y:     d.TargetY,
		Speed: d.Speed,
		Until: d.Deadline,
	}}
}

func (d *Duo) placeTarget() {
	for try := 0; try < 80; try++ {
		cx := 2 + d.randN(56-4)
		cy := 2 + d.randN(36-4)
		x, y := cellCenter(cx, cy)
		if d.blocked(x, y) {
			continue
		}
		if d.Index > 0 && dist(x, y, d.TargetX, d.TargetY) < 120 {
			continue
		}
		d.TargetX, d.TargetY = x, y
		d.Deadline = d.Frame + TargetFrames
		return
	}
	x, y := cellCenter(28, 8)
	d.TargetX, d.TargetY = x, y
	d.Deadline = d.Frame + TargetFrames
}

func (d *Duo) blocked(x, y float64) bool {
	for _, z := range d.Zones {
		need := z.R + TargetRadius + 16
		if dist(x, y, z.X, z.Y) < need {
			return true
		}
	}
	return false
}

func (d *Duo) finish(win bool, reason string) {
	d.done = true
	d.win = win
	d.reason = reason
}

func (d *Duo) stepPlayer(i int, op Op) {
	p := &d.players[i]
	if !p.alive {
		return
	}
	p.dashCD = math.Max(0, p.dashCD-FrameDT)
	p.dashT = math.Max(0, p.dashT-FrameDT)
	if op.Dash && p.dashCD <= 0 && p.dashT <= 0 {
		p.dashT = DashTime
		p.dashCD = DashCD
	}
	ax, ay := float64(op.Ax), float64(op.Ay)
	if ax != 0 && ay != 0 {
		inv := 1 / math.Sqrt2
		ax *= inv
		ay *= inv
	}
	speed := PlayerSpeed
	if p.dashT > 0 {
		speed *= DashMult
	}
	p.x = clamp(p.x+ax*speed*FrameDT, PlayerR, float64(WorldW)-PlayerR)
	p.y = clamp(p.y+ay*speed*FrameDT, PlayerR, float64(WorldH)-PlayerR)
}

func sanitizeOp(op Op) Op {
	op.Ax = clampInt(op.Ax, -1, 1)
	op.Ay = clampInt(op.Ay, -1, 1)
	return op
}

func (d *Duo) randN(n int) int {
	if n <= 0 {
		return 0
	}
	d.rng = d.rng*1664525 + 1013904223
	return int(d.rng>>16) % n
}

func dist(ax, ay, bx, by float64) float64 {
	dx, dy := ax-bx, ay-by
	return math.Hypot(dx, dy)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
