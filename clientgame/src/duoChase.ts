// 与 service/room/snake.go 同一套演算。格子走、追猎物/闲逛、不进安全区。
import { CELL, COLS, ROWS, WORLD_H, WORLD_W, cellCenter, clamp } from "./config";

export const SNAKE_COUNT = 2;
export const SNAKE_R = 16;
export const KILL_RADIUS = 25;
export const FRAME_DT = 0.05;
export const TARGET_RADIUS = 28;
export const PLAYER_R_DUO = 11;

export const ZONE_SAFE = "safe";
export const ZONE_START = "start";

const NEAR_DIST = CELL * 12;
const START_LEN = 12;
const TURN_LOCK = 1;
const PACE = 1 / 1.5;

export interface DuoZone {
  x: number;
  y: number;
  r: number;
  kind: string;
}

export interface DuoPlayer {
  roleid: string;
  x: number;
  y: number;
  alive: boolean;
}

export interface Head {
  x: number;
  y: number;
}

interface Cell {
  x: number;
  y: number;
}

interface Snake {
  body: Cell[];
  dirX: number;
  dirY: number;
  straight: number;
  aimPrey: boolean;
  aimID: number;
  wanderX: number;
  wanderY: number;
  wanderRetarget: number;
  decideIn: number;
  acc: number;
}

const SPOTS = [
  { x: 16, y: 14, dx: 1, dy: 0, wx: WORLD_W * 0.22, wy: WORLD_H * 0.3 },
  { x: 40, y: 10, dx: -1, dy: 0, wx: WORLD_W * 0.78, wy: WORLD_H * 0.22 },
];

export class Chase {
  private readonly snakes: Snake[] = [];
  private readonly zones: DuoZone[];
  private rng: number;

  constructor(seed: number, zones: DuoZone[]) {
    const s = (seed >>> 0) || 1;
    this.zones = zones.map((z) => ({ ...z }));
    this.rng = (s ^ 0x51ed270b) >>> 0;
    for (let i = 0; i < SNAKE_COUNT; i++) {
      const sp = SPOTS[i];
      const body: Cell[] = [];
      for (let k = 0; k < START_LEN; k++) {
        body.push({ x: sp.x - sp.dx * k, y: sp.y - sp.dy * k });
      }
      this.snakes.push({
        body,
        dirX: sp.dx,
        dirY: sp.dy,
        straight: TURN_LOCK,
        aimPrey: false,
        aimID: -2,
        wanderX: sp.wx,
        wanderY: sp.wy,
        wanderRetarget: 0,
        decideIn: this.take(1000) / 1000,
        acc: 0,
      });
    }
  }

  heads(): Head[] {
    return this.snakes.map((snake) => headOf(snake));
  }

  bodies(): Head[][] {
    return this.snakes.map((snake) =>
      snake.body.map((seg) => {
        const c = cellCenter(seg.x, seg.y);
        return { x: c.x, y: c.y };
      }),
    );
  }

  step(players: DuoPlayer[], speed: number): string[] {
    const mult = speed < 1 ? 1 : speed;
    for (const snake of this.snakes) this.tick(snake, players, mult);
    const dead: string[] = [];
    for (const p of players) {
      if (!p.alive || p.roleid === "" || protectedAt(this.zones, p.x, p.y)) continue;
      if (bitten(this.snakes, p.x, p.y)) dead.push(p.roleid);
    }
    return dead;
  }

  private tick(snake: Snake, players: DuoPlayer[], speed: number): void {
    snake.wanderRetarget = Math.max(0, snake.wanderRetarget - FRAME_DT);
    if (this.holds(snake, players)) {
      snake.decideIn = 1;
    } else if (snake.aimPrey) {
      snake.decideIn = 1;
      this.chooseAim(snake, players);
    } else {
      snake.decideIn -= FRAME_DT;
      if (snake.decideIn <= 0) {
        snake.decideIn += 1;
        if (snake.decideIn <= 0) snake.decideIn = 1;
        this.chooseAim(snake, players);
      }
    }
    let interval = (0.155 / speed) * PACE;
    if (interval < 0.04) interval = 0.04;
    snake.acc += FRAME_DT;
    let guard = 0;
    while (snake.acc >= interval && guard++ < 5) {
      snake.acc -= interval;
      this.act(snake, players);
    }
  }

  private holds(snake: Snake, players: DuoPlayer[]): boolean {
    if (!snake.aimPrey || snake.aimID < 0 || snake.aimID >= players.length) return false;
    const p = players[snake.aimID];
    return p.alive && !protectedAt(this.zones, p.x, p.y);
  }

  private chooseAim(snake: Snake, players: DuoPlayer[]): void {
    const exposed = exposedPrey(this.zones, players);
    if (exposed.length === 0) {
      this.wander(snake);
      return;
    }
    const h = headOf(snake);
    let nearest = exposed[0];
    let nearestD = Math.hypot(h.x - players[nearest].x, h.y - players[nearest].y);
    for (const id of exposed.slice(1)) {
      const d = Math.hypot(h.x - players[id].x, h.y - players[id].y);
      if (d < nearestD) {
        nearestD = d;
        nearest = id;
      }
    }
    const near = nearestD < NEAR_DIST;
    const roll = this.take(1000) / 1000;
    const nearestCut = near ? 0.6 : 0.4;
    const fartherCut = near ? 1 : 0.8;
    if (roll < nearestCut) {
      snake.aimPrey = true;
      snake.aimID = nearest;
      return;
    }
    if (roll < fartherCut) {
      const others = exposed.filter((id) => id !== nearest);
      if (others.length > 0) {
        snake.aimPrey = true;
        snake.aimID = others[this.take(others.length)];
        return;
      }
    }
    this.wander(snake);
  }

  private wander(snake: Snake): void {
    snake.aimPrey = false;
    snake.aimID = -2;
    snake.wanderRetarget = 0;
    this.ensureWander(snake);
  }

  private ensureWander(snake: Snake): void {
    const h = headOf(snake);
    const near =
      Math.hypot(h.x - snake.wanderX, h.y - snake.wanderY) < CELL * 2.5 || snake.wanderRetarget <= 0;
    if (!near) return;
    for (let guard = 0; guard < 24; guard++) {
      const x = CELL * 2 + this.take(WORLD_W - CELL * 4);
      const y = CELL * 2 + this.take(WORLD_H - CELL * 4);
      if (protectedAt(this.zones, x, y)) continue;
      if (Math.hypot(x - h.x, y - h.y) < CELL * 6) continue;
      snake.wanderX = x;
      snake.wanderY = y;
      snake.wanderRetarget = 3 + this.take(4000) / 1000;
      return;
    }
    snake.wanderX = clamp(WORLD_W - h.x, CELL * 2, WORLD_W - CELL * 2);
    snake.wanderY = clamp(WORLD_H - h.y, CELL * 2, WORLD_H - CELL * 2);
    snake.wanderRetarget = 3;
  }

  private huntFocus(snake: Snake, players: DuoPlayer[]): { x: number; y: number } {
    if (snake.aimPrey && snake.aimID >= 0 && snake.aimID < players.length) {
      const p = players[snake.aimID];
      if (p.alive && !protectedAt(this.zones, p.x, p.y)) {
        return { x: clamp(p.x, CELL, WORLD_W - CELL), y: clamp(p.y, CELL, WORLD_H - CELL) };
      }
    }
    this.ensureWander(snake);
    return { x: snake.wanderX, y: snake.wanderY };
  }

  private act(snake: Snake, players: DuoPlayer[]): void {
    let [dirX, dirY] = this.chooseDir(snake, players);
    if (dirX !== snake.dirX || dirY !== snake.dirY) snake.straight = 0;
    else snake.straight += 1;
    snake.dirX = dirX;
    snake.dirY = dirY;
    const head = snake.body[0];
    let nx = head.x + dirX;
    let ny = head.y + dirY;
    if (cellProtected(this.zones, nx, ny)) {
      [dirX, dirY] = this.chooseDirAvoid(snake, players);
      snake.dirX = dirX;
      snake.dirY = dirY;
      nx = head.x + dirX;
      ny = head.y + dirY;
      if (inMap(nx, ny) && !cellProtected(this.zones, nx, ny)) advance(snake, nx, ny);
      return;
    }
    if (!inMap(nx, ny)) {
      this.turnOpen(snake);
      return;
    }
    advance(snake, nx, ny);
  }

  private chooseDir(snake: Snake, players: DuoPlayer[]): [number, number] {
    const focus = this.huntFocus(snake, players);
    const head = snake.body[0];
    const canTurn = snake.straight >= TURN_LOCK;
    const list = sideDirs(snake.dirX, snake.dirY).map((d) => {
      const nx = head.x + d[0];
      const ny = head.y + d[1];
      const c = cellCenter(nx, ny);
      let pen = 0;
      if (cellProtected(this.zones, nx, ny)) {
        pen = exposedPrey(this.zones, players).length === 0 ? 200 : 80;
      }
      return {
        dx: d[0],
        dy: d[1],
        safe: inMap(nx, ny),
        dist: Math.hypot(c.x - focus.x, c.y - focus.y) + pen,
        fwd: d[0] === snake.dirX && d[1] === snake.dirY,
      };
    });
    let fwd = 0;
    for (let i = 0; i < list.length; i++) {
      if (list[i].fwd) {
        fwd = i;
        break;
      }
    }
    if (!canTurn) {
      if (list[fwd].safe) return [list[fwd].dx, list[fwd].dy];
      let best = fwd;
      for (let i = 0; i < list.length; i++) {
        if (list[i].safe && list[i].dist < list[best].dist) best = i;
      }
      if (list[best].safe) return [list[best].dx, list[best].dy];
      return [list[fwd].dx, list[fwd].dy];
    }
    let best = -1;
    for (let i = 0; i < list.length; i++) {
      if (!list[i].safe) continue;
      if (best < 0 || list[i].dist < list[best].dist) best = i;
    }
    if (best >= 0) return [list[best].dx, list[best].dy];
    return [list[fwd].dx, list[fwd].dy];
  }

  private chooseDirAvoid(snake: Snake, players: DuoPlayer[]): [number, number] {
    const focus = this.huntFocus(snake, players);
    const head = snake.body[0];
    let bestDX = snake.dirX;
    let bestDY = snake.dirY;
    let bestD = Number.POSITIVE_INFINITY;
    let found = false;
    for (const d of sideDirs(snake.dirX, snake.dirY)) {
      const nx = head.x + d[0];
      const ny = head.y + d[1];
      if (!inMap(nx, ny) || cellProtected(this.zones, nx, ny)) continue;
      const c = cellCenter(nx, ny);
      const dd = Math.hypot(c.x - focus.x, c.y - focus.y);
      if (!found || dd < bestD) {
        bestD = dd;
        bestDX = d[0];
        bestDY = d[1];
        found = true;
      }
    }
    return [bestDX, bestDY];
  }

  private turnOpen(snake: Snake): void {
    const head = snake.body[0];
    for (const d of sideDirs(snake.dirX, snake.dirY)) {
      const nx = head.x + d[0];
      const ny = head.y + d[1];
      if (inMap(nx, ny) && !cellProtected(this.zones, nx, ny)) {
        snake.dirX = d[0];
        snake.dirY = d[1];
        snake.straight = 0;
        return;
      }
    }
  }

  private take(n: number): number {
    if (n <= 0) return 0;
    this.rng = (Math.imul(this.rng, 1664525) + 1013904223) >>> 0;
    return ((this.rng >>> 16) % n) | 0;
  }
}

function headOf(snake: Snake): Head {
  if (!snake.body.length) return { x: 0, y: 0 };
  const c = cellCenter(snake.body[0].x, snake.body[0].y);
  return { x: c.x, y: c.y };
}

function advance(snake: Snake, nx: number, ny: number): void {
  snake.body.unshift({ x: nx, y: ny });
  if (snake.body.length > START_LEN) snake.body.length = START_LEN;
}

function inMap(x: number, y: number): boolean {
  return x >= 0 && y >= 0 && x < COLS && y < ROWS;
}

function sideDirs(dx: number, dy: number): Array<[number, number]> {
  const all: Array<[number, number]> = [
    [1, 0],
    [-1, 0],
    [0, 1],
    [0, -1],
  ];
  return all.filter((d) => !(d[0] === -dx && d[1] === -dy));
}

function protectedAt(zones: DuoZone[], x: number, y: number): boolean {
  for (const z of zones) {
    if (Math.hypot(x - z.x, y - z.y) <= z.r) return true;
  }
  return false;
}

function cellProtected(zones: DuoZone[], cx: number, cy: number): boolean {
  const c = cellCenter(cx, cy);
  return protectedAt(zones, c.x, c.y);
}

function exposedPrey(zones: DuoZone[], players: DuoPlayer[]): number[] {
  const out: number[] = [];
  for (let i = 0; i < players.length; i++) {
    const p = players[i];
    if (p.alive && p.roleid !== "" && !protectedAt(zones, p.x, p.y)) out.push(i);
  }
  return out;
}

function bitten(snakes: Snake[], x: number, y: number): boolean {
  for (const snake of snakes) {
    const h = headOf(snake);
    if (Math.hypot(h.x - x, h.y - y) <= KILL_RADIUS) return true;
  }
  return false;
}
