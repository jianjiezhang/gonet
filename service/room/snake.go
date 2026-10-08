package room

import "math"

const (
	snakeNearDist = Cell * 12
	snakeStartLen = 12
	snakeTurnLock = 1
	snakePace     = 1.0 / 1.5
)

// Head 是一条蛇这一帧的头（像素）。两边用同一份玩家位置各自算，结果应当相同。
type Head struct {
	X float64
	Y float64
}

type cell struct {
	x, y int
}

type snake struct {
	body           []cell
	dirX, dirY     int
	straight       int
	aimPrey        bool
	aimID          int
	wanderX        float64
	wanderY        float64
	wanderRetarget float64
	decideIn       float64
	acc            float64
}

// Chase 是两条蛇的本地演算。不在服务器上跑。规则对齐玩法 3：格子走、追猎物/闲逛、不进安全区。
type Chase struct {
	snakes [SnakeCount]snake
	zones  []Zone
	rng    uint32
}

// NewChase 按本场种子放两条蛇。随机序列和目标点的那条分开。
func NewChase(seed uint32, zones []Zone) *Chase {
	if seed == 0 {
		seed = 1
	}
	c := &Chase{
		zones: append([]Zone(nil), zones...),
		rng:   seed ^ 0x51ed270b,
	}
	spots := [SnakeCount]struct {
		x, y   int
		dx, dy int
		wx, wy float64
	}{
		{16, 14, 1, 0, float64(WorldW) * 0.22, float64(WorldH) * 0.3},
		{40, 10, -1, 0, float64(WorldW) * 0.78, float64(WorldH) * 0.22},
	}
	for i := 0; i < SnakeCount; i++ {
		sp := spots[i]
		body := make([]cell, snakeStartLen)
		for k := 0; k < snakeStartLen; k++ {
			body[k] = cell{x: sp.x - sp.dx*k, y: sp.y - sp.dy*k}
		}
		c.snakes[i] = snake{
			body:     body,
			dirX:     sp.dx,
			dirY:     sp.dy,
			straight: snakeTurnLock,
			aimID:    -2,
			wanderX:  sp.wx,
			wanderY:  sp.wy,
			decideIn: float64(randN(&c.rng, 1000)) / 1000,
		}
	}
	return c
}

// Heads 是当前两条蛇的头，顺序固定。
func (c *Chase) Heads() []Head {
	out := make([]Head, SnakeCount)
	for i := range c.snakes {
		out[i] = headOf(&c.snakes[i])
	}
	return out
}

// Bodies 是每条蛇身体各节的像素中心，给画面用。
func (c *Chase) Bodies() [][]Head {
	out := make([][]Head, SnakeCount)
	for i := range c.snakes {
		body := c.snakes[i].body
		seg := make([]Head, len(body))
		for k := range body {
			x, y := cellCenter(body[k].x, body[k].y)
			seg[k] = Head{X: x, Y: y}
		}
		out[i] = seg
	}
	return out
}

// Step 用这一帧服务器广播的玩家位置推进蛇。speed 是当前目标阶段的倍数。
// 返回这一帧被咬到的 roleid。安全区（含出发区）里的人不被追、也不被咬；蛇也不进安全区。
func (c *Chase) Step(players []PlayerState, speed float64) []string {
	if speed < 1 {
		speed = 1
	}
	for i := range c.snakes {
		c.tick(&c.snakes[i], players, speed)
	}
	var dead []string
	for i := range players {
		p := players[i]
		if !p.Alive || p.RoleID == "" || protected(c.zones, p.X, p.Y) {
			continue
		}
		if bitten(c.snakes, p.X, p.Y) {
			dead = append(dead, p.RoleID)
		}
	}
	return dead
}

func (c *Chase) tick(s *snake, players []PlayerState, speed float64) {
	s.wanderRetarget = math.Max(0, s.wanderRetarget-FrameDT)
	if c.holds(s, players) {
		s.decideIn = 1
	} else if s.aimPrey {
		s.decideIn = 1
		c.chooseAim(s, players)
	} else {
		s.decideIn -= FrameDT
		if s.decideIn <= 0 {
			s.decideIn += 1
			if s.decideIn <= 0 {
				s.decideIn = 1
			}
			c.chooseAim(s, players)
		}
	}

	interval := (0.155 / speed) * snakePace
	if interval < 0.04 {
		interval = 0.04
	}
	s.acc += FrameDT
	for guard := 0; s.acc >= interval && guard < 5; guard++ {
		s.acc -= interval
		c.act(s, players)
	}
}

func (c *Chase) holds(s *snake, players []PlayerState) bool {
	if !s.aimPrey || s.aimID < 0 || s.aimID >= len(players) {
		return false
	}
	p := players[s.aimID]
	return p.Alive && !protected(c.zones, p.X, p.Y)
}

func (c *Chase) chooseAim(s *snake, players []PlayerState) {
	exposed := exposedPrey(c.zones, players)
	if len(exposed) == 0 {
		c.wander(s)
		return
	}
	h := headOf(s)
	nearest := exposed[0]
	nearestD := dist(h.X, h.Y, players[nearest].X, players[nearest].Y)
	for _, id := range exposed[1:] {
		d := dist(h.X, h.Y, players[id].X, players[id].Y)
		if d < nearestD {
			nearestD = d
			nearest = id
		}
	}
	near := nearestD < snakeNearDist
	roll := float64(randN(&c.rng, 1000)) / 1000
	nearestCut := 0.4
	fartherCut := 0.8
	if near {
		nearestCut = 0.6
		fartherCut = 1
	}
	if roll < nearestCut {
		s.aimPrey = true
		s.aimID = nearest
		return
	}
	if roll < fartherCut {
		others := make([]int, 0, len(exposed))
		for _, id := range exposed {
			if id != nearest {
				others = append(others, id)
			}
		}
		if len(others) > 0 {
			s.aimPrey = true
			s.aimID = others[randN(&c.rng, len(others))]
			return
		}
	}
	c.wander(s)
}

func (c *Chase) wander(s *snake) {
	s.aimPrey = false
	s.aimID = -2
	s.wanderRetarget = 0
	c.ensureWander(s)
}

func (c *Chase) ensureWander(s *snake) {
	h := headOf(s)
	near := dist(h.X, h.Y, s.wanderX, s.wanderY) < float64(Cell)*2.5 || s.wanderRetarget <= 0
	if !near {
		return
	}
	for guard := 0; guard < 24; guard++ {
		x := float64(Cell*2) + float64(randN(&c.rng, WorldW-Cell*4))
		y := float64(Cell*2) + float64(randN(&c.rng, WorldH-Cell*4))
		if protected(c.zones, x, y) {
			continue
		}
		if dist(x, y, h.X, h.Y) < float64(Cell)*6 {
			continue
		}
		s.wanderX = x
		s.wanderY = y
		s.wanderRetarget = 3 + float64(randN(&c.rng, 4000))/1000
		return
	}
	s.wanderX = clamp(float64(WorldW)-h.X, float64(Cell*2), float64(WorldW-Cell*2))
	s.wanderY = clamp(float64(WorldH)-h.Y, float64(Cell*2), float64(WorldH-Cell*2))
	s.wanderRetarget = 3
}

func (c *Chase) huntFocus(s *snake, players []PlayerState) (float64, float64) {
	if s.aimPrey && s.aimID >= 0 && s.aimID < len(players) {
		p := players[s.aimID]
		if p.Alive && !protected(c.zones, p.X, p.Y) {
			return clamp(p.X, float64(Cell), float64(WorldW-Cell)), clamp(p.Y, float64(Cell), float64(WorldH-Cell))
		}
	}
	c.ensureWander(s)
	return s.wanderX, s.wanderY
}

func (c *Chase) act(s *snake, players []PlayerState) {
	dirX, dirY := c.chooseDir(s, players)
	if dirX != s.dirX || dirY != s.dirY {
		s.straight = 0
	} else {
		s.straight++
	}
	s.dirX, s.dirY = dirX, dirY
	head := s.body[0]
	nx, ny := head.x+dirX, head.y+dirY
	if cellProtected(c.zones, nx, ny) {
		ax, ay := c.chooseDirAvoid(s, players)
		s.dirX, s.dirY = ax, ay
		nx, ny = head.x+ax, head.y+ay
		if inMap(nx, ny) && !cellProtected(c.zones, nx, ny) {
			advance(s, nx, ny)
		}
		return
	}
	if !inMap(nx, ny) {
		c.turnOpen(s)
		return
	}
	advance(s, nx, ny)
}

func (c *Chase) chooseDir(s *snake, players []PlayerState) (int, int) {
	tx, ty := c.huntFocus(s, players)
	head := s.body[0]
	canTurn := s.straight >= snakeTurnLock
	type cand struct {
		dx, dy int
		safe   bool
		dist   float64
		fwd    bool
	}
	list := make([]cand, 0, 3)
	for _, d := range sideDirs(s.dirX, s.dirY) {
		nx, ny := head.x+d[0], head.y+d[1]
		cx, cy := cellCenter(nx, ny)
		pen := 0.0
		if cellProtected(c.zones, nx, ny) {
			if len(exposedPrey(c.zones, players)) == 0 {
				pen = 200
			} else {
				pen = 80
			}
		}
		list = append(list, cand{
			dx: d[0], dy: d[1],
			safe: inMap(nx, ny),
			dist: math.Hypot(cx-tx, cy-ty) + pen,
			fwd:  d[0] == s.dirX && d[1] == s.dirY,
		})
	}
	fwd := 0
	for i := range list {
		if list[i].fwd {
			fwd = i
			break
		}
	}
	if !canTurn {
		if list[fwd].safe {
			return list[fwd].dx, list[fwd].dy
		}
		best := fwd
		for i := range list {
			if list[i].safe && list[i].dist < list[best].dist {
				best = i
			}
		}
		if list[best].safe {
			return list[best].dx, list[best].dy
		}
		return list[fwd].dx, list[fwd].dy
	}
	best := -1
	for i := range list {
		if !list[i].safe {
			continue
		}
		if best < 0 || list[i].dist < list[best].dist {
			best = i
		}
	}
	if best >= 0 {
		return list[best].dx, list[best].dy
	}
	return list[fwd].dx, list[fwd].dy
}

func (c *Chase) chooseDirAvoid(s *snake, players []PlayerState) (int, int) {
	tx, ty := c.huntFocus(s, players)
	head := s.body[0]
	bestDX, bestDY := s.dirX, s.dirY
	bestD := math.MaxFloat64
	found := false
	for _, d := range sideDirs(s.dirX, s.dirY) {
		nx, ny := head.x+d[0], head.y+d[1]
		if !inMap(nx, ny) || cellProtected(c.zones, nx, ny) {
			continue
		}
		cx, cy := cellCenter(nx, ny)
		dd := math.Hypot(cx-tx, cy-ty)
		if !found || dd < bestD {
			bestD = dd
			bestDX, bestDY = d[0], d[1]
			found = true
		}
	}
	return bestDX, bestDY
}

func (c *Chase) turnOpen(s *snake) {
	head := s.body[0]
	for _, d := range sideDirs(s.dirX, s.dirY) {
		nx, ny := head.x+d[0], head.y+d[1]
		if inMap(nx, ny) && !cellProtected(c.zones, nx, ny) {
			s.dirX, s.dirY = d[0], d[1]
			s.straight = 0
			return
		}
	}
}

func exposedPrey(zones []Zone, players []PlayerState) []int {
	var out []int
	for i := range players {
		p := players[i]
		if p.Alive && p.RoleID != "" && !protected(zones, p.X, p.Y) {
			out = append(out, i)
		}
	}
	return out
}

func headOf(s *snake) Head {
	if len(s.body) == 0 {
		return Head{}
	}
	x, y := cellCenter(s.body[0].x, s.body[0].y)
	return Head{X: x, Y: y}
}

func advance(s *snake, nx, ny int) {
	s.body = append([]cell{{x: nx, y: ny}}, s.body...)
	if len(s.body) > snakeStartLen {
		s.body = s.body[:snakeStartLen]
	}
}

func inMap(x, y int) bool {
	cols, rows := WorldW/Cell, WorldH/Cell
	return x >= 0 && y >= 0 && x < cols && y < rows
}

func sideDirs(dx, dy int) [][2]int {
	all := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	out := make([][2]int, 0, 3)
	for _, d := range all {
		if d[0] == -dx && d[1] == -dy {
			continue
		}
		out = append(out, d)
	}
	return out
}

func protected(zones []Zone, x, y float64) bool {
	for i := range zones {
		z := zones[i]
		if dist(x, y, z.X, z.Y) <= z.R {
			return true
		}
	}
	return false
}

func cellProtected(zones []Zone, cx, cy int) bool {
	x, y := cellCenter(cx, cy)
	return protected(zones, x, y)
}

func bitten(snakes [SnakeCount]snake, x, y float64) bool {
	for i := range snakes {
		h := headOf(&snakes[i])
		if dist(h.X, h.Y, x, y) <= KillRadius {
			return true
		}
	}
	return false
}

func randN(rng *uint32, n int) int {
	if n <= 0 {
		return 0
	}
	*rng = *rng*1664525 + 1013904223
	return int(*rng>>16) % n
}
