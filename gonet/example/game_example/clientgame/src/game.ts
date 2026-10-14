import {
  BOOST_MULT,
  BOOST_TIME,
  BODY_R,
  CELL,
  COLS,
  DASH_CD,
  DASH_MULT,
  DASH_TIME,
  KILL_R,
  LUNGE_CD,
  LUNGE_RANGE,
  LUNGE_RECOVER,
  LUNGE_STEP_TIME,
  LUNGE_STEPS,
  LUNGE_WINDUP,
  ANGER_TIME,
  MAX_LEN,
  MIN_LEN,
  PLAYER_R,
  PLAYER_SPEED,
  ROWS,
  SAVE_KEY,
  START_LEN,
  TURN_LOCK,
  VIEWPORT_H,
  VIEWPORT_W,
  VIEW_H,
  VIEW_W,
  ORIGIN_X,
  ORIGIN_Y,
  WORLD_H,
  WORLD_W,
  cellCenter,
  clamp,
  type Dir,
} from "./config";

export interface Input {
  ax: number;
  ay: number;
  spaceEdge: boolean;
  enterEdge: boolean;
  restartEdge: boolean;
  clickEdge: boolean;
}

/** 屏幕布局。全屏玩法只改这块，世界地图格子数和 CELL 不变。 */
export interface ViewLayout {
  screenW: number;
  screenH: number;
  originX: number;
  originY: number;
  viewportW: number;
  viewportH: number;
  wide: boolean;
}

export function windowView(): ViewLayout {
  return {
    screenW: VIEW_W,
    screenH: VIEW_H,
    originX: ORIGIN_X,
    originY: ORIGIN_Y,
    viewportW: VIEWPORT_W,
    viewportH: VIEWPORT_H,
    wide: false,
  };
}

/** 视野等于整张世界地图。格子数和 CELL 不变，镜头不再裁切。 */
export function wideView(): ViewLayout {
  return {
    screenW: WORLD_W,
    screenH: WORLD_H,
    originX: 0,
    originY: 0,
    viewportW: WORLD_W,
    viewportH: WORLD_H,
    wide: true,
  };
}

export interface Cell {
  x: number;
  y: number;
}

interface Fruit {
  x: number;
  y: number;
  ttl: number;
}

interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  life: number;
  max: number;
  color: string;
  r: number;
}

interface Floater {
  x: number;
  y: number;
  text: string;
  life: number;
}

export type SnakeMode = "chase" | "windup" | "lunge" | "recover";

interface Snake {
  body: Cell[];
  dir: Dir;
  grow: number;
  /** 自上次转弯后已直走格数。 */
  straight: number;
  mode: SnakeMode;
  /** windup / recover 剩余秒；lunge 用 steps。 */
  modeT: number;
  lungeLeft: number;
  lungeCd: number;
  /** 扑咬期间玩家是否曾处在头的侧向危险带（用于判定躲开）。 */
  dodgeChance: boolean;
  dodged: boolean;
  /** 躲开后短暂狂暴，追击更快。 */
  anger: number;
}

interface Player {
  x: number;
  y: number;
  vx: number;
  vy: number;
  facingX: number;
  facingY: number;
  dash: number;
  dashCd: number;
  boost: number;
  trail: { x: number; y: number }[];
}

export interface Game {
  phase: "title" | "play" | "dead";
  time: number;
  score: number;
  scoreAcc: number;
  bestTime: number;
  bestScore: number;
  snake: Snake;
  player: Player;
  fruits: Fruit[];
  snakeAcc: number;
  growAcc: number;
  nearCd: number;
  shake: number;
  flash: number;
  seed: number;
  particles: Particle[];
  floaters: Floater[];
  camX: number;
  camY: number;
  view: ViewLayout;
}

const LEFT: Dir = { x: -1, y: 0 };
const RIGHT: Dir = { x: 1, y: 0 };
const UP: Dir = { x: 0, y: -1 };
const DOWN: Dir = { x: 0, y: 1 };
const ALL_DIRS: Dir[] = [UP, DOWN, LEFT, RIGHT];

export function createGame(view: ViewLayout = windowView()): Game {
  const game: Game = {
    phase: "title",
    time: 0,
    score: 0,
    scoreAcc: 0,
    bestTime: 0,
    bestScore: 0,
    snake: emptySnake(),
    player: emptyPlayer(),
    fruits: [],
    snakeAcc: 0,
    growAcc: 0,
    nearCd: 0,
    shake: 0,
    flash: 0,
    seed: (Date.now() ^ 0x9e3779b9) >>> 0,
    particles: [],
    floaters: [],
    camX: 0,
    camY: 0,
    view,
  };
  loadBest(game);
  layout(game);
  return game;
}

export function setGameView(game: Game, view: ViewLayout): void {
  game.view = view;
  updateCamera(game);
}

function emptySnake(): Snake {
  return {
    body: [],
    dir: { ...RIGHT },
    grow: 0,
    straight: TURN_LOCK,
    mode: "chase",
    modeT: 0,
    lungeLeft: 0,
    lungeCd: 1.2,
    dodgeChance: false,
    dodged: false,
    anger: 0,
  };
}

function emptyPlayer(): Player {
  return {
    x: 0,
    y: 0,
    vx: 0,
    vy: 0,
    facingX: -1,
    facingY: 0,
    dash: 0,
    dashCd: 0,
    boost: 0,
    trail: [],
  };
}

export function update(game: Game, input: Input, dt: number): void {
  dt = Math.min(Math.max(dt, 0), 0.05);
  decayFx(game, dt);
  updateCamera(game);

  if (game.phase !== "play") {
    if (wantsStart(game, input)) {
      if (game.phase === "dead") restart(game);
      else game.phase = "play";
    }
    return;
  }

  game.time += dt;
  game.scoreAcc += dt;
  while (game.scoreAcc >= 1) {
    game.scoreAcc -= 1;
    game.score += 10;
  }
  game.growAcc += dt;
  if (game.growAcc >= 6) {
    game.growAcc -= 6;
    if (game.snake.body.length < MAX_LEN) game.snake.grow += 1;
  }

  stepPlayer(game, input, dt);
  if (game.phase !== "play") return;
  stepSnake(game, dt);
  if (game.phase !== "play") return;
  pickupFruits(game);
  ageFruits(game, dt);
  noteNearMiss(game, dt);
  updateCamera(game);
}

function wantsStart(game: Game, input: Input): boolean {
  if (input.enterEdge || input.clickEdge) return true;
  if (game.phase === "dead" && input.restartEdge) return true;
  if (game.phase !== "play" && input.spaceEdge) return true;
  return false;
}

function restart(game: Game): void {
  const bestTime = game.bestTime;
  const bestScore = game.bestScore;
  game.seed = (game.seed + 997) >>> 0;
  layout(game);
  game.bestTime = bestTime;
  game.bestScore = bestScore;
  game.phase = "play";
}

function layout(game: Game): void {
  game.time = 0;
  game.score = 0;
  game.scoreAcc = 0;
  game.snakeAcc = 0;
  game.growAcc = 0;
  game.nearCd = 0;
  game.shake = 0;
  game.flash = 0;
  game.particles = [];
  game.floaters = [];
  game.fruits = [];

  const body: Cell[] = [];
  const sx = 10;
  const sy = Math.floor(ROWS / 2);
  for (let i = 0; i < START_LEN; i++) body.push({ x: sx - i, y: sy });
  game.snake = {
    body,
    dir: { ...RIGHT },
    grow: 0,
    straight: TURN_LOCK,
    mode: "chase",
    modeT: 0,
    lungeLeft: 0,
    lungeCd: 0.8,
    dodgeChance: false,
    dodged: false,
    anger: 0,
  };

  const start = cellCenter(COLS - 12, Math.floor(ROWS / 2));
  game.player = {
    x: start.x,
    y: start.y,
    vx: 0,
    vy: 0,
    facingX: -1,
    facingY: 0,
    dash: 0,
    dashCd: 0,
    boost: 0,
    trail: [],
  };

  let left = 10;
  let guard = 0;
  while (left > 0 && guard++ < 800) {
    const x = Math.floor(rand(game) * COLS);
    const y = Math.floor(rand(game) * ROWS);
    if (occupiedBodyOrFruit(game, x, y)) continue;
    const pc = pixelToCell(game.player.x, game.player.y);
    if (Math.abs(pc.x - x) + Math.abs(pc.y - y) < 6) continue;
    game.fruits.push({ x, y, ttl: -1 });
    left -= 1;
  }

  updateCamera(game);
}

function updateCamera(game: Game): void {
  const vw = game.view.viewportW;
  const vh = game.view.viewportH;
  const maxX = WORLD_W - vw;
  const maxY = WORLD_H - vh;
  if (maxX <= 0) game.camX = maxX / 2;
  else game.camX = clamp(game.player.x - vw / 2, 0, maxX);
  if (maxY <= 0) game.camY = maxY / 2;
  else game.camY = clamp(game.player.y - vh / 2, 0, maxY);
}

function stepPlayer(game: Game, input: Input, dt: number): void {
  const player = game.player;
  let ax = input.ax;
  let ay = input.ay;
  const len = Math.hypot(ax, ay);
  if (len > 0) {
    ax /= len;
    ay /= len;
    player.facingX = ax;
    player.facingY = ay;
  }

  if (input.spaceEdge && player.dashCd <= 0 && player.dash <= 0) {
    player.dash = DASH_TIME;
    player.dashCd = DASH_CD;
    if (len === 0 && player.facingX === 0 && player.facingY === 0) {
      player.facingX = 1;
    }
  }

  let speed = PLAYER_SPEED;
  if (player.boost > 0) speed *= BOOST_MULT;
  if (player.dash > 0) speed *= DASH_MULT;

  const dx = (len > 0 ? ax : player.dash > 0 ? player.facingX : 0) * speed * dt;
  const dy = (len > 0 ? ay : player.dash > 0 ? player.facingY : 0) * speed * dt;
  const dist = Math.hypot(dx, dy);
  const steps = Math.max(1, Math.ceil(dist / 6));
  const prevX = player.x;
  const prevY = player.y;

  for (let i = 0; i < steps; i++) {
    const moved = movePlayer(game, dx / steps, dy / steps);
    player.x = moved.x;
    player.y = moved.y;
    trackLungeDodge(game);
    if (headHits(game)) {
      player.vx = (player.x - prevX) / dt;
      player.vy = (player.y - prevY) / dt;
      die(game);
      return;
    }
  }

  player.vx = (player.x - prevX) / dt;
  player.vy = (player.y - prevY) / dt;
  player.dash = Math.max(0, player.dash - dt);
  player.dashCd = Math.max(0, player.dashCd - dt);
  player.boost = Math.max(0, player.boost - dt);

  if (player.dash > 0) {
    player.trail.push({ x: player.x, y: player.y });
    if (player.trail.length > 7) player.trail.shift();
  } else if (player.trail.length > 0) {
    player.trail.shift();
  }
}

function movePlayer(game: Game, dx: number, dy: number): { x: number; y: number } {
  let x = game.player.x + dx;
  let y = game.player.y + dy;
  const minX = PLAYER_R + 2;
  const maxX = WORLD_W - PLAYER_R - 2;
  const minY = PLAYER_R + 2;
  const maxY = WORLD_H - PLAYER_R - 2;

  for (let n = 0; n < 5; n++) {
    x = clamp(x, minX, maxX);
    y = clamp(y, minY, maxY);

    const body = game.snake.body;
    for (let i = 1; i < body.length; i++) {
      const c = cellCenter(body[i].x, body[i].y);
      pushOut(x, y, c.x, c.y, PLAYER_R + BODY_R - 1, (nx, ny) => {
        x = nx;
        y = ny;
      });
    }
  }
  return {
    x: clamp(x, minX, maxX),
    y: clamp(y, minY, maxY),
  };
}

function pushOut(
  x: number,
  y: number,
  cx: number,
  cy: number,
  min: number,
  apply: (nx: number, ny: number) => void,
): void {
  let ox = x - cx;
  let oy = y - cy;
  let d = Math.hypot(ox, oy);
  if (d >= min) return;
  if (d < 0.001) {
    ox = 1;
    oy = 0;
    d = 1;
  }
  const push = (min - d) / d;
  apply(x + ox * push, y + oy * push);
}

function stepSnake(game: Game, dt: number): void {
  const snake = game.snake;
  snake.lungeCd = Math.max(0, snake.lungeCd - dt);
  snake.anger = Math.max(0, snake.anger - dt);

  if (snake.mode === "recover") {
    snake.modeT -= dt;
    game.snakeAcc = 0;
    if (snake.modeT <= 0) {
      snake.mode = "chase";
      snake.modeT = 0;
      snake.straight = TURN_LOCK;
    }
    if (headHits(game)) die(game);
    return;
  }

  if (snake.mode === "windup") {
    snake.modeT -= dt;
    game.snakeAcc = 0;
    // 方向在进入 windup 时已锁定，预警阶段不再改瞄，玩家才能读线躲开
    if (snake.modeT <= 0) {
      snake.mode = "lunge";
      snake.lungeLeft = LUNGE_STEPS;
      snake.dodgeChance = false;
      snake.dodged = false;
      snake.straight = 0;
      game.shake = Math.max(game.shake, 3);
    }
    if (headHits(game)) die(game);
    return;
  }

  if (snake.mode === "chase" && snake.lungeCd <= 0 && headDistance(game) < LUNGE_RANGE) {
    const aim = desiredPoint(game);
    const head = snake.body[0];
    const hc = cellCenter(head.x, head.y);
    snake.dir = bestCardinal(aim.x - hc.x, aim.y - hc.y, snake.dir);
    snake.mode = "windup";
    snake.modeT = LUNGE_WINDUP;
    snake.dodgeChance = false;
    snake.dodged = false;
    game.snakeAcc = 0;
    return;
  }

  game.snakeAcc += dt;
  const step = snake.mode === "lunge" ? LUNGE_STEP_TIME : snakeInterval(game);
  let guard = 0;
  while (game.snakeAcc >= step && guard++ < 5 && game.phase === "play") {
    game.snakeAcc -= step;
    snakeAct(game);
    const mode = game.snake.mode;
    if (mode === "recover" || mode === "windup") {
      game.snakeAcc = 0;
      break;
    }
    if (headHits(game)) {
      die(game);
      return;
    }
  }
}

function snakeAct(game: Game): void {
  const snake = game.snake;
  let dir = snake.dir;

  if (snake.mode === "lunge") {
    dir = snake.dir;
  } else {
    dir = chooseDir(game);
    if (dir.x !== snake.dir.x || dir.y !== snake.dir.y) {
      snake.straight = 0;
    } else {
      snake.straight += 1;
    }
  }
  snake.dir = dir;

  const head = snake.body[0];
  const nx = head.x + dir.x;
  const ny = head.y + dir.y;

  if (!isSafe(game, nx, ny)) {
    if (nx < 0 || ny < 0 || nx >= COLS || ny >= ROWS) {
      crashWall(game);
    } else {
      const hit = indexAt(snake, nx, ny, false);
      crashBody(game, hit < 0 ? MIN_LEN : hit);
    }
    endLungeIfAny(game, false);
    return;
  }

  snake.body.unshift({ x: nx, y: ny });
  const ate = takeFruitAt(game, nx, ny);
  if (ate && snake.body.length + snake.grow < MAX_LEN) snake.grow += 2;
  if (snake.body.length >= MAX_LEN) snake.grow = 0;
  if (snake.grow > 0 && snake.body.length < MAX_LEN) snake.grow -= 1;
  else snake.body.pop();

  if (snake.mode === "lunge") {
    snake.lungeLeft -= 1;
    trackLungeDodge(game);
    if (snake.lungeLeft <= 0) {
      endLungeIfAny(game, true);
    }
  }
}

function endLungeIfAny(game: Game, finished: boolean): void {
  const snake = game.snake;
  if (snake.mode !== "lunge" && snake.mode !== "windup") return;

  const reward = snake.mode === "lunge" && finished && snake.dodgeChance && !snake.dodged;
  snake.mode = "recover";
  snake.modeT = LUNGE_RECOVER;
  snake.lungeLeft = 0;
  snake.lungeCd = LUNGE_CD;
  if (reward) {
    snake.dodged = true;
    snake.anger = ANGER_TIME;
    snake.lungeCd = Math.min(snake.lungeCd, 0.55);
    game.score += 180;
    game.shake = Math.max(game.shake, 4);
    const h = headPixel(game);
    game.floaters.push({ x: h.x, y: h.y - 24, text: "躲开！+180", life: 1.05 });
    burst(game, h, "#9ad7ff", 12);
  }
}

/**
 * 扑咬中：若头从身侧擦过（近但未咬中），记为可奖励的躲开。
 */
function trackLungeDodge(game: Game): void {
  const snake = game.snake;
  if (snake.mode !== "lunge" || snake.dodged) return;
  const d = headDistance(game);
  if (d < KILL_R + 38 && d > KILL_R) {
    const h = headPixel(game);
    const along =
      (game.player.x - h.x) * snake.dir.x + (game.player.y - h.y) * snake.dir.y;
    // 在头前方或刚过身边，不算「侧闪」；侧向才算
    if (along < CELL * 0.8) snake.dodgeChance = true;
  }
}

function chooseDir(game: Game): Dir {
  const target = desiredPoint(game);
  const head = game.snake.body[0];
  const facing = game.snake.dir;
  const canTurn = game.snake.straight >= TURN_LOCK;
  const close = headDistance(game) < LUNGE_RANGE * 1.15;
  // 贴身时少偏爱直行，更容易抄截；远了才更爱冲直线
  const straightBias = close ? 8 : 26;

  const candidates = sideOptions(facing).map((dir) => {
    const nx = head.x + dir.x;
    const ny = head.y + dir.y;
    const center = cellCenter(nx, ny);
    return {
      dir,
      safe: isSafe(game, nx, ny),
      dist: Math.hypot(center.x - target.x, center.y - target.y),
      straight: dir.x === facing.x && dir.y === facing.y,
    };
  });

  const forward =
    candidates.find((c) => c.dir.x === facing.x && c.dir.y === facing.y) ?? candidates[0];

  if (!canTurn) {
    if (forward.safe) return forward.dir;
    const safe = candidates.filter((c) => c.safe);
    if (safe.length === 0) return forward.dir;
    safe.sort((a, b) => a.dist - b.dist);
    return safe[0].dir;
  }

  const safe = candidates.filter((c) => c.safe);
  if (safe.length === 0) return forward.dir;

  let bestSafeDist = Infinity;
  for (const item of safe) bestSafeDist = Math.min(bestSafeDist, item.dist);

  if (!forward.safe && forward.dist < bestSafeDist + 70) return forward.dir;

  let best = safe[0];
  let bestCost = Infinity;
  for (const item of safe) {
    const cost = item.dist - (item.straight ? straightBias : 0);
    if (cost < bestCost) {
      bestCost = cost;
      best = item;
    }
  }
  return best.dir;
}

function desiredPoint(game: Game): { x: number; y: number } {
  const p = game.player;
  // 追击时多抄截你前方；预警锁定用中等预判
  const look = game.snake.mode === "windup" ? 0.28 : 0.48;
  return {
    x: clamp(p.x + p.vx * look, CELL, WORLD_W - CELL),
    y: clamp(p.y + p.vy * look, CELL, WORLD_H - CELL),
  };
}

function bestCardinal(dx: number, dy: number, fallback: Dir): Dir {
  if (Math.abs(dx) > Math.abs(dy)) return dx > 0 ? RIGHT : LEFT;
  if (Math.abs(dy) > 0.001) return dy > 0 ? DOWN : UP;
  return fallback;
}

function crashWall(game: Game): void {
  const snake = game.snake;
  const extra = snake.body.length - MIN_LEN;
  if (extra >= 2) {
    const drop = Math.max(2, Math.floor(extra / 2));
    dropTail(game, snake.body.length - drop);
    game.score += 100;
    floaterAtHead(game, "撞墙 +100");
  } else {
    floaterAtHead(game, "撞墙");
  }
  enterRecover(game, 0.55);
  turnToOpen(game);
  game.shake = Math.max(game.shake, 5);
  game.flash = Math.max(game.flash, 0.12);
  burst(game, headPixel(game), "#9ad7ff", 10);
}

function crashBody(game: Game, hit: number): void {
  const snake = game.snake;
  const cutAt = hit < MIN_LEN ? Math.max(MIN_LEN, snake.body.length - 3) : hit;
  if (cutAt < snake.body.length && cutAt >= MIN_LEN) dropTail(game, cutAt);
  enterRecover(game, 0.9);
  turnToOpen(game);
  game.score += 220;
  game.shake = Math.max(game.shake, 8);
  game.flash = Math.max(game.flash, 0.2);
  floaterAtHead(game, "断身 +220");
  burst(game, headPixel(game), "#b6ff9a", 16);
}

function enterRecover(game: Game, t: number): void {
  const snake = game.snake;
  const fromLunge = snake.mode === "lunge" || snake.mode === "windup";
  snake.mode = "recover";
  snake.modeT = Math.max(t, fromLunge ? LUNGE_RECOVER : t);
  snake.lungeLeft = 0;
  snake.lungeCd = Math.max(snake.lungeCd, LUNGE_CD * 0.6);
  snake.grow = 0;
  snake.straight = TURN_LOCK;
}

function dropTail(game: Game, cutAt: number): void {
  const dropped = game.snake.body.splice(cutAt);
  for (const seg of dropped) {
    if (game.fruits.length >= 36) break;
    if (game.fruits.some((f) => f.x === seg.x && f.y === seg.y)) continue;
    game.fruits.push({ x: seg.x, y: seg.y, ttl: 14 });
  }
}

function turnToOpen(game: Game): void {
  const target = desiredPoint(game);
  const head = game.snake.body[0];
  let best: Dir | null = null;
  let bestDist = Infinity;
  for (const dir of ALL_DIRS) {
    const nx = head.x + dir.x;
    const ny = head.y + dir.y;
    if (!isSafe(game, nx, ny)) continue;
    const c = cellCenter(nx, ny);
    const dist = Math.hypot(c.x - target.x, c.y - target.y);
    if (dist < bestDist) {
      bestDist = dist;
      best = dir;
    }
  }
  if (best) game.snake.dir = best;
}

function sideOptions(dir: Dir): Dir[] {
  return ALL_DIRS.filter((d) => !(d.x === -dir.x && d.y === -dir.y));
}

function isSafe(game: Game, x: number, y: number): boolean {
  if (x < 0 || y < 0 || x >= COLS || y >= ROWS) return false;
  const ignoreTail = game.snake.grow === 0;
  return indexAt(game.snake, x, y, ignoreTail) < 0;
}

function indexAt(snake: Snake, x: number, y: number, ignoreTail: boolean): number {
  const last = snake.body.length - (ignoreTail ? 1 : 0);
  for (let i = 0; i < last; i++) {
    if (snake.body[i].x === x && snake.body[i].y === y) return i;
  }
  return -1;
}

function occupiedBodyOrFruit(game: Game, x: number, y: number): boolean {
  if (game.snake.body.some((c) => c.x === x && c.y === y)) return true;
  return game.fruits.some((f) => f.x === x && f.y === y);
}

function takeFruitAt(game: Game, x: number, y: number): boolean {
  const idx = game.fruits.findIndex((f) => f.x === x && f.y === y);
  if (idx < 0) return false;
  game.fruits.splice(idx, 1);
  return true;
}

function pickupFruits(game: Game): void {
  const p = game.player;
  for (let i = game.fruits.length - 1; i >= 0; i--) {
    const fruit = game.fruits[i];
    const c = cellCenter(fruit.x, fruit.y);
    if (Math.hypot(p.x - c.x, p.y - c.y) <= PLAYER_R + 12) {
      game.fruits.splice(i, 1);
      game.score += 100;
      game.player.boost = BOOST_TIME;
      game.floaters.push({ x: c.x, y: c.y, text: "加速 +100", life: 0.8 });
      burst(game, c, "#ffd15c", 8);
    }
  }
}

function ageFruits(game: Game, dt: number): void {
  game.fruits = game.fruits.filter((fruit) => {
    if (fruit.ttl < 0) return true;
    fruit.ttl -= dt;
    return fruit.ttl > 0;
  });
}

function noteNearMiss(game: Game, dt: number): void {
  game.nearCd = Math.max(0, game.nearCd - dt);
  if (game.snake.mode === "lunge") return;
  const d = headDistance(game);
  if (d < 56 && d > KILL_R && game.nearCd <= 0) {
    game.nearCd = 0.75;
    game.score += 15;
    game.shake = Math.max(game.shake, 2);
    const h = headPixel(game);
    game.floaters.push({ x: h.x, y: h.y - 18, text: "惊险 +15", life: 0.6 });
  }
}

function headHits(game: Game): boolean {
  return headDistance(game) < KILL_R;
}

function headDistance(game: Game): number {
  const h = headPixel(game);
  return Math.hypot(game.player.x - h.x, game.player.y - h.y);
}

function headPixel(game: Game): { x: number; y: number } {
  const head = game.snake.body[0];
  return cellCenter(head.x, head.y);
}

function die(game: Game): void {
  if (game.phase !== "play") return;
  game.phase = "dead";
  game.shake = 12;
  game.flash = 0.45;
  const h = headPixel(game);
  burst(game, h, "#ff6b6b", 18);
  burst(game, { x: game.player.x, y: game.player.y }, "#7ad7ff", 12);
  if (game.time > game.bestTime) game.bestTime = game.time;
  if (game.score > game.bestScore) game.bestScore = game.score;
  saveBest(game);
}

function floaterAtHead(game: Game, text: string): void {
  const h = headPixel(game);
  game.floaters.push({ x: h.x, y: h.y - 20, text, life: 0.9 });
}

function burst(game: Game, at: { x: number; y: number }, color: string, count: number): void {
  for (let i = 0; i < count; i++) {
    if (game.particles.length > 90) game.particles.shift();
    const a = rand(game) * Math.PI * 2;
    const s = 30 + rand(game) * 110;
    game.particles.push({
      x: at.x,
      y: at.y,
      vx: Math.cos(a) * s,
      vy: Math.sin(a) * s,
      life: 0.35 + rand(game) * 0.25,
      max: 0.6,
      color,
      r: 2 + rand(game) * 2.5,
    });
  }
}

function decayFx(game: Game, dt: number): void {
  game.shake = Math.max(0, game.shake - dt * 30);
  game.flash = Math.max(0, game.flash - dt * 1.4);
  for (const p of game.particles) {
    p.life -= dt;
    p.x += p.vx * dt;
    p.y += p.vy * dt;
    p.vy += 40 * dt;
  }
  game.particles = game.particles.filter((p) => p.life > 0);
  for (const f of game.floaters) {
    f.life -= dt;
    f.y -= 28 * dt;
  }
  game.floaters = game.floaters.filter((f) => f.life > 0);
}

function snakeInterval(game: Game): number {
  const k = clamp(game.time / 40, 0, 1);
  let interval = 0.152 - 0.055 * k;
  if (game.snake.anger > 0) interval *= 0.72;
  // 贴身追击再加快一点，避免大地图上永远赶不上
  if (headDistance(game) < LUNGE_RANGE * 1.4) interval *= 0.9;
  return interval;
}

function pixelToCell(px: number, py: number): Cell {
  return {
    x: clamp(Math.floor(px / CELL), 0, COLS - 1),
    y: clamp(Math.floor(py / CELL), 0, ROWS - 1),
  };
}

function rand(game: Game): number {
  game.seed = (Math.imul(game.seed, 1664525) + 1013904223) >>> 0;
  return game.seed / 4294967296;
}

function loadBest(game: Game): void {
  try {
    const raw = localStorage.getItem(SAVE_KEY);
    if (!raw) return;
    const data = JSON.parse(raw) as { bestTime?: number; bestScore?: number };
    if (typeof data.bestTime === "number") game.bestTime = data.bestTime;
    if (typeof data.bestScore === "number") game.bestScore = data.bestScore;
  } catch {
    /* ignore */
  }
}

function saveBest(game: Game): void {
  try {
    localStorage.setItem(
      SAVE_KEY,
      JSON.stringify({ bestTime: game.bestTime, bestScore: game.bestScore }),
    );
  } catch {
    /* ignore */
  }
}

export function formatTime(t: number): string {
  const m = Math.floor(t / 60);
  const s = Math.floor(t % 60);
  const cs = Math.floor((t % 1) * 10);
  return `${m}:${s.toString().padStart(2, "0")}.${cs}`;
}

/** 世界坐标 → 屏幕坐标。 */
export function worldToScreen(game: Game, wx: number, wy: number): { x: number; y: number } {
  return {
    x: wx - game.camX + game.view.originX,
    y: wy - game.camY + game.view.originY,
  };
}
