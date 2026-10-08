import {
  ANGER_TIME,
  BODY_R,
  CELL,
  COLS,
  DASH_CD,
  DASH_MULT,
  DASH_TIME,
  KILL_R,
  LUNGE_CD,
  LUNGE_RECOVER,
  LUNGE_STEP_TIME,
  LUNGE_STEPS,
  MAX_LEN,
  MIN_LEN,
  PLAYER_R,
  PLAYER_SPEED,
  ROWS,
  START_LEN,
  TURN_LOCK,
  WORLD_H,
  WORLD_W,
  cellCenter,
  clamp,
  type Dir,
} from "./config";
import { windowView, type ViewLayout } from "./game";
import {
  ARENA_BEACON_TIME,
  ARENA_SAFES,
  ARENA_TARGET_COUNT,
  START_RADIUS,
  BEACON_RADIUS,
  BEACON_STAND,
  BOT_COUNT,
  MISSION_SAVE_KEY,
  MISSION_TIME,
  SAFE_CANDIDATES,
  SAFE_COLLAPSE,
  SAFE_COOLDOWN,
  SAFE_RADIUS,
  SAFE_SHELTER,
  SAFE_UNSTABLE,
  defaultBeacons,
  dist,
} from "./missionConfig";

export interface Input {
  ax: number;
  ay: number;
  spaceEdge: boolean;
  enterEdge: boolean;
  restartEdge: boolean;
  clickEdge: boolean;
  escapeEdge: boolean;
}

interface Cell {
  x: number;
  y: number;
}

type SnakeMode = "chase" | "windup" | "lunge" | "recover";
export type SafePhase = "ready" | "shelter" | "unstable" | "collapse" | "cooldown";

interface Snake {
  body: Cell[];
  dir: Dir;
  grow: number;
  straight: number;
  mode: SnakeMode;
  modeT: number;
  lungeLeft: number;
  lungeCd: number;
  dodgeChance: boolean;
  dodged: boolean;
  anger: number;
  /** 撞身后短暂穿身，避免卡死连撞。 */
  phaseThrough: number;
  wanderX: number;
  wanderY: number;
  wanderRetarget: number;
  acc: number;
}

interface Mover {
  x: number;
  y: number;
  vx: number;
  vy: number;
  facingX: number;
  facingY: number;
  dash: number;
  dashCd: number;
  alive: boolean;
  /** -1 = 玩家 */
  botId: number;
  beaconIndex: number;
  standT: number;
  finishTime: number;
  color: string;
}

export interface SafeZone {
  x: number;
  y: number;
  phase: SafePhase;
  phaseT: number;
  candidateIndex: number;
  radius: number;
  /** 玩法 3 底部开始区。 */
  start?: boolean;
}

export interface Beacon {
  x: number;
  y: number;
  name: string;
}

export interface MissionGame {
  phase: "brief" | "play" | "win" | "dead" | "timeout";
  elapsed: number;
  timeLeft: number;
  score: number;
  bestScore: number;
  snake: Snake;
  snakes: Snake[];
  player: Mover;
  bots: Mover[];
  beacons: Beacon[];
  playerBeacon: number;
  playerStand: number;
  /** 玩法 3：当前目标剩余秒数；玩法 2 不用。 */
  objectiveTimeLeft: number;
  safes: SafeZone[];
  snakeAcc: number;
  growAcc: number;
  shake: number;
  flash: number;
  seed: number;
  camX: number;
  camY: number;
  view: ViewLayout;
  floaters: { x: number; y: number; text: string; life: number }[];
  particles: {
    x: number;
    y: number;
    vx: number;
    vy: number;
    life: number;
    max: number;
    color: string;
    r: number;
  }[];
  introT: number;
}

const LEFT: Dir = { x: -1, y: 0 };
const RIGHT: Dir = { x: 1, y: 0 };
const UP: Dir = { x: 0, y: -1 };
const DOWN: Dir = { x: 0, y: 1 };
const ALL_DIRS: Dir[] = [UP, DOWN, LEFT, RIGHT];

const BOT_COLORS = ["#f0a0ff", "#a0f0c0", "#f0d080", "#80d0f0"];

export function createMission(view: ViewLayout = windowView()): MissionGame {
  const game: MissionGame = {
    phase: "brief",
    elapsed: 0,
    timeLeft: MISSION_TIME,
    score: 0,
    bestScore: 0,
    snake: emptySnake(),
    snakes: [],
    player: emptyMover(-1, "#7ad7ff"),
    bots: [],
    beacons: defaultBeacons(),
    playerBeacon: 0,
    playerStand: 0,
    objectiveTimeLeft: ARENA_BEACON_TIME,
    safes: [],
    snakeAcc: 0,
    growAcc: 0,
    shake: 0,
    flash: 0,
    seed: (Date.now() ^ 0xabcdef) >>> 0,
    camX: 0,
    camY: 0,
    view,
    floaters: [],
    particles: [],
    introT: 5,
  };
  loadBest(game);
  layout(game);
  return game;
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
    lungeCd: 1.0,
    dodgeChance: false,
    dodged: false,
    anger: 0,
    phaseThrough: 0,
    wanderX: WORLD_W / 2,
    wanderY: WORLD_H / 2,
    wanderRetarget: 0,
    acc: 0,
  };
}

function emptyMover(botId: number, color: string): Mover {
  return {
    x: 0,
    y: 0,
    vx: 0,
    vy: 0,
    facingX: 1,
    facingY: 0,
    dash: 0,
    dashCd: 0,
    alive: true,
    botId,
    beaconIndex: 0,
    standT: 0,
    finishTime: -1,
    color,
  };
}

export function updateMission(game: MissionGame, input: Input, dt: number): boolean {
  dt = Math.min(Math.max(dt, 0), 0.05);
  decayFx(game, dt);
  updateCamera(game);

  if (input.escapeEdge) return true;

  if (game.phase === "brief") {
    // 必须手动确认，不自动开局
    if (input.enterEdge || input.clickEdge || input.spaceEdge) {
      game.phase = "play";
      game.introT = 0;
    }
    return false;
  }

  if (game.phase !== "play") {
    if (input.enterEdge || input.clickEdge || input.restartEdge) {
      const best = game.bestScore;
      const seed = (game.seed + 911) >>> 0;
      const view = game.view;
      Object.assign(game, createMission(view));
      game.bestScore = best;
      game.seed = seed;
      layout(game);
      game.phase = "play";
    }
    return false;
  }

  game.elapsed += dt;
  if (game.view.wide) {
    game.objectiveTimeLeft = Math.max(0, game.objectiveTimeLeft - dt);
    game.timeLeft = game.objectiveTimeLeft;
  } else {
    game.timeLeft = Math.max(0, MISSION_TIME - game.elapsed);
  }
  if (game.timeLeft <= 0) {
    game.phase = "timeout";
    saveBest(game);
    return false;
  }

  stepSafes(game, dt);
  stepPlayer(game, input, dt);
  if (game.phase !== "play") return false;
  stepBots(game, dt);
  stepSnake(game, dt);
  if (game.phase !== "play") return false;
  stepObjectives(game, dt);
  updateCamera(game);
  return false;
}

function layout(game: MissionGame): void {
  game.elapsed = 0;
  game.timeLeft = game.view.wide ? ARENA_BEACON_TIME : MISSION_TIME;
  game.objectiveTimeLeft = ARENA_BEACON_TIME;
  game.score = 0;
  game.snakeAcc = 0;
  game.growAcc = 0;
  game.shake = 0;
  game.flash = 0;
  game.floaters = [];
  game.particles = [];
  game.playerBeacon = 0;
  game.playerStand = 0;
  game.introT = 5;

  const wide = game.view.wide;
  const spots = wide
    ? [
        { x: 16, y: 14, dir: RIGHT, wx: WORLD_W * 0.22, wy: WORLD_H * 0.3 },
        { x: 40, y: 10, dir: LEFT, wx: WORLD_W * 0.78, wy: WORLD_H * 0.22 },
        { x: 12, y: 22, dir: UP, wx: WORLD_W * 0.3, wy: WORLD_H * 0.78 },
        { x: 48, y: 24, dir: DOWN, wx: WORLD_W * 0.72, wy: WORLD_H * 0.68 },
      ]
    : [{ x: 12, y: 10, dir: RIGHT, wx: WORLD_W * 0.3, wy: WORLD_H * 0.4 }];
  game.snakes = spots.map((spot) => placeSnake(spot.x, spot.y, spot.dir, spot.wx, spot.wy));
  game.snake = game.snakes[0];

  const used = new Set<number>();
  game.safes = [];
  if (wide) {
    ARENA_SAFES.forEach((c, idx) => {
      game.safes.push({
        x: c.x,
        y: c.y,
        phase: "shelter",
        phaseT: Number.POSITIVE_INFINITY,
        candidateIndex: idx,
        radius: SAFE_RADIUS,
      });
    });
    game.safes.push({
      x: WORLD_W / 2,
      y: WORLD_H - 72,
      phase: "shelter",
      phaseT: Number.POSITIVE_INFINITY,
      candidateIndex: -1,
      radius: START_RADIUS,
      start: true,
    });
  } else {
    for (let i = 0; i < 2; i++) {
      let idx = Math.floor(rand(game) * SAFE_CANDIDATES.length);
      let guard = 0;
      while (used.has(idx) && guard++ < 40) idx = Math.floor(rand(game) * SAFE_CANDIDATES.length);
      used.add(idx);
      const c = SAFE_CANDIDATES[idx];
      game.safes.push({
        x: c.x,
        y: c.y,
        phase: "ready",
        phaseT: 0,
        candidateIndex: idx,
        radius: SAFE_RADIUS,
      });
    }
  }

  game.beacons = wide ? [rollArenaBeacon(game, "1")] : defaultBeacons();

  if (wide) {
    spawnInStart(game);
  } else {
    const p = cellCenter(40, 22);
    game.player = {
      ...emptyMover(-1, "#7ad7ff"),
      x: p.x,
      y: p.y,
      facingX: -1,
    };
    game.bots = [];
    for (let i = 0; i < BOT_COUNT; i++) {
      const c = cellCenter(36 + (i % 3) * 3, 20 + Math.floor(i / 3) * 4);
      game.bots.push({
        ...emptyMover(i, BOT_COLORS[i] ?? "#ccc"),
        x: c.x + (rand(game) - 0.5) * 20,
        y: c.y + (rand(game) - 0.5) * 20,
        facingX: -1,
      });
    }
  }

  updateCamera(game);
}

function spawnInStart(game: MissionGame): void {
  const zone = game.safes.find((item) => item.start);
  const cx = zone?.x ?? WORLD_W / 2;
  const cy = zone?.y ?? WORLD_H - 72;
  const offsets = [-120, -60, 0, 60, 120];
  const at = (i: number) => ({
    x: clamp(cx + offsets[i], PLAYER_R + 2, WORLD_W - PLAYER_R - 2),
    y: clamp(cy, PLAYER_R + 2, WORLD_H - PLAYER_R - 2),
  });
  const playerAt = at(2);
  game.player = {
    ...emptyMover(-1, "#7ad7ff"),
    x: playerAt.x,
    y: playerAt.y,
    facingX: -1,
  };
  game.bots = [];
  const slots = [0, 1, 3, 4];
  for (let i = 0; i < BOT_COUNT; i++) {
    const spot = at(slots[i]);
    game.bots.push({
      ...emptyMover(i, BOT_COLORS[i] ?? "#ccc"),
      x: spot.x,
      y: spot.y,
      facingX: -1,
    });
  }
}

function rollArenaBeacon(game: MissionGame, name: string): Beacon {
  for (let i = 0; i < 48; i++) {
    const x = 96 + rand(game) * (WORLD_W - 192);
    const y = 96 + rand(game) * (WORLD_H - 220);
    const blocked = game.safes.some((zone) => dist(x, y, zone.x, zone.y) < zone.radius + BEACON_RADIUS + 16);
    if (!blocked) return { x, y, name };
  }
  return { x: WORLD_W * 0.5, y: WORLD_H * 0.35, name };
}

function placeSnake(x: number, y: number, dir: Dir, wanderX: number, wanderY: number): Snake {
  const body: Cell[] = [];
  for (let i = 0; i < START_LEN; i++) body.push({ x: x - dir.x * i, y: y - dir.y * i });
  return {
    ...emptySnake(),
    body,
    dir: { ...dir },
    lungeCd: 1.2,
    wanderX,
    wanderY,
  };
}

function stepSafes(game: MissionGame, dt: number): void {
  if (game.view.wide) return;
  for (const zone of game.safes) {
    const occupied = someoneInSafe(game, zone) && (zone.phase === "ready" || zone.phase === "shelter" || zone.phase === "unstable");

    if (zone.phase === "ready") {
      if (occupied) {
        zone.phase = "shelter";
        zone.phaseT = SAFE_SHELTER;
      }
      continue;
    }

    zone.phaseT -= dt;
    if (zone.phaseT > 0) continue;

    if (zone.phase === "shelter") {
      zone.phase = "unstable";
      zone.phaseT = SAFE_UNSTABLE;
    } else if (zone.phase === "unstable") {
      zone.phase = "collapse";
      zone.phaseT = SAFE_COLLAPSE;
    } else if (zone.phase === "collapse") {
      zone.phase = "cooldown";
      zone.phaseT = SAFE_COOLDOWN;
    } else if (zone.phase === "cooldown") {
      relocateSafe(game, zone);
      zone.phase = "ready";
      zone.phaseT = 0;
    }
  }
}

function relocateSafe(game: MissionGame, zone: SafeZone): void {
  const taken = new Set(game.safes.map((s) => s.candidateIndex));
  const options = SAFE_CANDIDATES.map((_, i) => i).filter((i) => !taken.has(i) || i === zone.candidateIndex);
  let idx = options[Math.floor(rand(game) * options.length)] ?? 0;
  // 尽量换位置
  if (options.length > 1) {
    const others = options.filter((i) => i !== zone.candidateIndex);
    if (others.length) idx = others[Math.floor(rand(game) * others.length)];
  }
  zone.candidateIndex = idx;
  zone.x = SAFE_CANDIDATES[idx].x;
  zone.y = SAFE_CANDIDATES[idx].y;
}

function someoneInSafe(game: MissionGame, zone: SafeZone): boolean {
  if (!zoneProtects(zone)) {
    // ready 也算可进入触发
    if (zone.phase !== "ready") return false;
  }
  if (game.player.alive && dist(game.player.x, game.player.y, zone.x, zone.y) <= zone.radius) return true;
  for (const bot of game.bots) {
    if (bot.alive && dist(bot.x, bot.y, zone.x, zone.y) <= zone.radius) return true;
  }
  return false;
}

export function zoneProtects(zone: SafeZone): boolean {
  return zone.phase === "shelter" || zone.phase === "unstable" || zone.phase === "ready";
}

export function isProtectedAt(game: MissionGame, x: number, y: number): boolean {
  for (const zone of game.safes) {
    if (!zoneProtects(zone)) continue;
    if (zone.phase === "ready") continue; // ready 未激活时不提供庇护，只是可进入
    if (dist(x, y, zone.x, zone.y) <= zone.radius) return true;
  }
  return false;
}

function stepPlayer(game: MissionGame, input: Input, dt: number): void {
  const player = game.player;
  if (!player.alive) return;
  moveMover(game, player, input.ax, input.ay, input.spaceEdge, dt, true);
  if (headHitsMover(game, player) && !isProtectedAt(game, player.x, player.y)) {
    player.alive = false;
    game.phase = "dead";
    game.shake = 12;
    game.flash = 0.4;
    burst(game, { x: player.x, y: player.y }, "#7ad7ff", 14);
    saveBest(game);
  }
}

function stepBots(game: MissionGame, dt: number): void {
  for (const bot of game.bots) {
    if (!bot.alive) continue;
    const steer = botSteer(game, bot);
    moveMover(game, bot, steer.x, steer.y, false, dt, false);
    if (headHitsMover(game, bot) && !isProtectedAt(game, bot.x, bot.y)) {
      bot.alive = false;
      burst(game, { x: bot.x, y: bot.y }, bot.color, 10);
      game.floaters.push({ x: bot.x, y: bot.y - 16, text: "猎物倒下", life: 0.8 });
    }
  }
}

function botSteer(game: MissionGame, bot: Mover): { x: number; y: number } {
  const head = nearestSnakeHead(game, bot.x, bot.y);
  const toSnake = dist(bot.x, bot.y, head.x, head.y);
  if (toSnake < 160) {
    const safe = nearestProtectingSafe(game, bot.x, bot.y);
    if (safe && toSnake < 200) {
      return norm(safe.x - bot.x, safe.y - bot.y);
    }
    return norm(bot.x - head.x, bot.y - head.y);
  }
  if (bot.finishTime >= 0 || (game.view.wide && game.playerBeacon >= ARENA_TARGET_COUNT)) {
    const safe = nearestProtectingSafe(game, bot.x, bot.y);
    if (safe) return norm(safe.x - bot.x, safe.y - bot.y);
    return { x: 0, y: 0 };
  }
  const beacon = game.view.wide
    ? game.beacons[0]
    : game.beacons[Math.min(bot.beaconIndex, game.beacons.length - 1)];
  if (!beacon) return { x: 0, y: 0 };
  const n = norm(beacon.x - bot.x, beacon.y - bot.y);
  // 轻微噪声，避免叠成一条线
  return norm(n.x + (rand(game) - 0.5) * 0.35, n.y + (rand(game) - 0.5) * 0.35);
}

function nearestProtectingSafe(game: MissionGame, x: number, y: number): SafeZone | null {
  let best: SafeZone | null = null;
  let bestD = Infinity;
  for (const zone of game.safes) {
    if (zone.phase === "cooldown" || zone.phase === "collapse") continue;
    const d = dist(x, y, zone.x, zone.y);
    if (d < bestD) {
      bestD = d;
      best = zone;
    }
  }
  return best;
}

function norm(x: number, y: number): { x: number; y: number } {
  const l = Math.hypot(x, y);
  if (l < 0.001) return { x: 0, y: 0 };
  return { x: x / l, y: y / l };
}

function moveMover(
  game: MissionGame,
  m: Mover,
  ax: number,
  ay: number,
  dashEdge: boolean,
  dt: number,
  isPlayer: boolean,
): void {
  let len = Math.hypot(ax, ay);
  if (len > 1) {
    ax /= len;
    ay /= len;
    len = 1;
  }
  if (len > 0) {
    m.facingX = ax;
    m.facingY = ay;
  }
  if (isPlayer && dashEdge && m.dashCd <= 0 && m.dash <= 0) {
    m.dash = DASH_TIME;
    m.dashCd = DASH_CD;
  }

  let speed = isPlayer ? PLAYER_SPEED : PLAYER_SPEED * 0.88;
  if (m.dash > 0) speed *= DASH_MULT;
  const dx = (len > 0 ? ax : m.dash > 0 ? m.facingX : 0) * speed * dt;
  const dy = (len > 0 ? ay : m.dash > 0 ? m.facingY : 0) * speed * dt;
  const prevX = m.x;
  const prevY = m.y;
  const steps = Math.max(1, Math.ceil(Math.hypot(dx, dy) / 6));
  for (let i = 0; i < steps; i++) {
    let x = m.x + dx / steps;
    let y = m.y + dy / steps;
    x = clamp(x, PLAYER_R + 2, WORLD_W - PLAYER_R - 2);
    y = clamp(y, PLAYER_R + 2, WORLD_H - PLAYER_R - 2);
    for (const snake of game.snakes) {
      for (let s = 1; s < snake.body.length; s++) {
        const c = cellCenter(snake.body[s].x, snake.body[s].y);
        const min = PLAYER_R + BODY_R - 1;
        const ox = x - c.x;
        const oy = y - c.y;
        const d = Math.hypot(ox, oy);
        if (d < min && d > 0.001) {
          const push = (min - d) / d;
          x += ox * push;
          y += oy * push;
        }
      }
    }
    m.x = clamp(x, PLAYER_R + 2, WORLD_W - PLAYER_R - 2);
    m.y = clamp(y, PLAYER_R + 2, WORLD_H - PLAYER_R - 2);
  }
  m.vx = (m.x - prevX) / dt;
  m.vy = (m.y - prevY) / dt;
  m.dash = Math.max(0, m.dash - dt);
  m.dashCd = Math.max(0, m.dashCd - dt);
}

function stepObjectives(game: MissionGame, dt: number): void {
  if (game.view.wide) {
    advanceSharedBeacon(game);
    return;
  }
  advanceObjective(game, game.player, true, dt);
  for (const bot of game.bots) {
    if (bot.alive) advanceObjective(game, bot, false, dt);
  }
}

function advanceSharedBeacon(game: MissionGame): void {
  if (game.playerBeacon >= ARENA_TARGET_COUNT) return;
  const beacon = game.beacons[0];
  if (!beacon) return;
  const last = game.playerBeacon === ARENA_TARGET_COUNT - 1;
  const preys = [game.player, ...game.bots.filter((bot) => bot.alive)];
  for (const prey of preys) {
    if (dist(prey.x, prey.y, beacon.x, beacon.y) > BEACON_RADIUS) continue;
    if (last && isProtectedAt(game, prey.x, prey.y)) continue;
    game.playerBeacon += 1;
    game.score += 200;
    const done = game.playerBeacon >= ARENA_TARGET_COUNT;
    game.floaters.push({
      x: beacon.x,
      y: beacon.y - 20,
      text: done ? "任务完成！" : `目标 ${beacon.name} ✓`,
      life: 1,
    });
    burst(game, beacon, "#ffd15c", 12);
    if (done) {
      prey.finishTime = game.elapsed;
      game.beacons = [];
      game.phase = "win";
      game.score += Math.floor(game.timeLeft * 2);
      saveBest(game);
    } else {
      game.beacons = [rollArenaBeacon(game, String(game.playerBeacon + 1))];
      game.objectiveTimeLeft = ARENA_BEACON_TIME;
      game.timeLeft = ARENA_BEACON_TIME;
    }
    return;
  }
}

function advanceObjective(game: MissionGame, m: Mover, isPlayer: boolean, dt: number): void {
  if (m.finishTime >= 0) return;
  if (m.beaconIndex >= game.beacons.length) return;
  const beacon = game.beacons[m.beaconIndex];
  // 最后一站不可在保护区内完成
  const last = m.beaconIndex === game.beacons.length - 1;
  if (last && isProtectedAt(game, m.x, m.y)) {
    if (isPlayer) game.playerStand = 0;
    else m.standT = 0;
    return;
  }
  if (dist(m.x, m.y, beacon.x, beacon.y) <= BEACON_RADIUS) {
    if (isPlayer) {
      game.playerStand += dt;
      if (game.playerStand >= BEACON_STAND) {
        game.playerStand = 0;
        game.playerBeacon += 1;
        m.beaconIndex = game.playerBeacon;
        game.score += 200;
        game.floaters.push({
          x: beacon.x,
          y: beacon.y - 20,
          text: m.beaconIndex >= game.beacons.length ? "任务完成！" : `信标 ${beacon.name} ✓`,
          life: 1,
        });
        burst(game, beacon, "#ffd15c", 12);
        if (m.beaconIndex >= game.beacons.length) {
          m.finishTime = game.elapsed;
          game.phase = "win";
          game.score += Math.floor(game.timeLeft * 2);
          saveBest(game);
        }
      }
    } else {
      m.standT += dt;
      if (m.standT >= BEACON_STAND) {
        m.standT = 0;
        m.beaconIndex += 1;
        if (m.beaconIndex >= game.beacons.length) {
          m.finishTime = game.elapsed;
          game.floaters.push({ x: m.x, y: m.y - 18, text: "机器人送达", life: 0.9 });
          game.phase = "win";
          saveBest(game);
        }
      }
    }
  } else if (isPlayer) {
    game.playerStand = 0;
  } else {
    m.standT = 0;
  }
}

function stepSnake(game: MissionGame, dt: number): void {
  for (const snake of game.snakes) {
    game.snake = snake;
    tickSnake(game, snake, dt);
    if (game.phase !== "play") return;
  }
}

function tickSnake(game: MissionGame, snake: Snake, dt: number): void {
  snake.lungeCd = Math.max(0, snake.lungeCd - dt);
  snake.anger = Math.max(0, snake.anger - dt);
  snake.phaseThrough = Math.max(0, snake.phaseThrough - dt);
  snake.wanderRetarget = Math.max(0, snake.wanderRetarget - dt);

  if (snake.mode === "recover") {
    snake.modeT -= dt;
    snake.acc = 0;
    if (snake.modeT <= 0) {
      snake.mode = "chase";
      snake.straight = TURN_LOCK;
      // 硬直结束若仍卡在身体上，再给一点穿身离开
      if (!headCellClear(game)) snake.phaseThrough = Math.max(snake.phaseThrough, 1.2);
    }
    checkSnakeKills(game);
    return;
  }

  if (snake.mode === "windup") {
    snake.modeT -= dt;
    snake.acc = 0;
    if (snake.modeT <= 0) {
      snake.mode = "lunge";
      snake.lungeLeft = LUNGE_STEPS;
      snake.dodgeChance = false;
      snake.dodged = false;
      snake.straight = 0;
      game.shake = Math.max(game.shake, 3);
    }
    checkSnakeKills(game);
    return;
  }

  snake.acc += dt;
  const pace = game.view.wide ? 1 / 1.5 : 1;
  const step = (snake.mode === "lunge" ? LUNGE_STEP_TIME : snakeInterval(game)) * pace;
  let guard = 0;
  while (snake.acc >= step && guard++ < 5 && game.phase === "play") {
    snake.acc -= step;
    snakeAct(game);
    const modeAfter: SnakeMode = game.snake.mode;
    if (modeAfter === "recover" || modeAfter === "windup") {
      snake.acc = 0;
      break;
    }
    checkSnakeKills(game);
    if (game.phase !== "play") return;
  }
}

/** 不在保护区内的活着的猎物（玩家+机器人）。 */
function exposedPrey(game: MissionGame): Mover[] {
  const list: Mover[] = [];
  if (game.player.alive && !isProtectedAt(game, game.player.x, game.player.y)) {
    list.push(game.player);
  }
  for (const bot of game.bots) {
    if (bot.alive && !isProtectedAt(game, bot.x, bot.y)) list.push(bot);
  }
  return list;
}

function huntFocus(game: MissionGame): { x: number; y: number } {
  const prey = exposedPrey(game);
  if (prey.length === 0) {
    ensureWanderTarget(game);
    return { x: game.snake.wanderX, y: game.snake.wanderY };
  }

  const h = headPixel(game);
  const look = 0.35;
  let best = prey[0];
  let bestD = Infinity;
  for (const m of prey) {
    const px = m.x + m.vx * look;
    const py = m.y + m.vy * look;
    const d = dist(h.x, h.y, px, py);
    if (d < bestD) {
      bestD = d;
      best = m;
    }
  }
  return {
    x: clamp(best.x + best.vx * look, CELL, WORLD_W - CELL),
    y: clamp(best.y + best.vy * look, CELL, WORLD_H - CELL),
  };
}

function ensureWanderTarget(game: MissionGame): void {
  const snake = game.snake;
  const h = headPixel(game);
  const near =
    dist(h.x, h.y, snake.wanderX, snake.wanderY) < CELL * 2.5 || snake.wanderRetarget <= 0;
  if (!near) return;

  let guard = 0;
  while (guard++ < 24) {
    const x = CELL * 2 + rand(game) * (WORLD_W - CELL * 4);
    const y = CELL * 2 + rand(game) * (WORLD_H - CELL * 4);
    // 不要把闲逛点设在安全区里，免得围着区转
    if (isProtectedAt(game, x, y)) continue;
    if (dist(x, y, h.x, h.y) < CELL * 6) continue;
    snake.wanderX = x;
    snake.wanderY = y;
    snake.wanderRetarget = 3 + rand(game) * 4;
    return;
  }
  snake.wanderX = clamp(WORLD_W - h.x, CELL * 2, WORLD_W - CELL * 2);
  snake.wanderY = clamp(WORLD_H - h.y, CELL * 2, WORLD_H - CELL * 2);
  snake.wanderRetarget = 3;
}

function snakeAct(game: MissionGame): void {
  const snake = game.snake;
  let dir = snake.dir;
  if (snake.mode !== "lunge") {
    dir = chooseDir(game);
    if (dir.x !== snake.dir.x || dir.y !== snake.dir.y) snake.straight = 0;
    else snake.straight += 1;
  }
  snake.dir = dir;
  const head = snake.body[0];
  const nx = head.x + dir.x;
  const ny = head.y + dir.y;

  // 庇护区内蛇不愿踏入（预警/扑咬除外可冲进去）
  if (snake.mode === "chase" && cellProtected(game, nx, ny)) {
    const alt = chooseDirAvoidSafe(game);
    snake.dir = alt;
    const ax = head.x + alt.x;
    const ay = head.y + alt.y;
    if (isSafe(game, ax, ay) && !cellProtected(game, ax, ay)) {
      advanceSnake(game, ax, ay);
      return;
    }
  }

  if (!isSafe(game, nx, ny)) {
    if (nx < 0 || ny < 0 || nx >= COLS || ny >= ROWS) crashWall(game);
    else {
      const hit = indexAt(snake, nx, ny, false);
      crashBody(game, hit < 0 ? MIN_LEN : hit);
    }
    endLunge(game, false);
    return;
  }

  advanceSnake(game, nx, ny);
  if (snake.mode === "lunge") {
    snake.lungeLeft -= 1;
    if (snake.lungeLeft <= 0) endLunge(game, true);
  }
}

function advanceSnake(game: MissionGame, nx: number, ny: number): void {
  const snake = game.snake;
  snake.body.unshift({ x: nx, y: ny });
  if (snake.grow > 0 && snake.body.length < MAX_LEN) snake.grow -= 1;
  else snake.body.pop();
}

function chooseDir(game: MissionGame): Dir {
  const target = huntFocus(game);
  const head = game.snake.body[0];
  const facing = game.snake.dir;
  const canTurn = game.snake.straight >= TURN_LOCK;
  const candidates = sideOptions(facing).map((dir) => {
    const nx = head.x + dir.x;
    const ny = head.y + dir.y;
    const center = cellCenter(nx, ny);
    let penalty = 0;
    if (cellProtected(game, nx, ny)) penalty += exposedPrey(game).length === 0 ? 200 : 80;
    return {
      dir,
      safe: isSafe(game, nx, ny),
      dist: Math.hypot(center.x - target.x, center.y - target.y) + penalty,
      straight: dir.x === facing.x && dir.y === facing.y,
    };
  });
  const forward = candidates.find((c) => c.straight) ?? candidates[0];
  if (!canTurn) {
    if (forward.safe) return forward.dir;
    const safe = candidates.filter((c) => c.safe).sort((a, b) => a.dist - b.dist);
    return safe[0]?.dir ?? forward.dir;
  }
  const safe = candidates.filter((c) => c.safe);
  if (!safe.length) return forward.dir;
  let best = safe[0];
  let bestCost = Infinity;
  for (const item of safe) {
    const cost = item.dist - (item.straight ? 18 : 0);
    if (cost < bestCost) {
      bestCost = cost;
      best = item;
    }
  }
  return best.dir;
}

function chooseDirAvoidSafe(game: MissionGame): Dir {
  return chooseDir(game);
}

function cellProtected(game: MissionGame, cx: number, cy: number): boolean {
  const p = cellCenter(cx, cy);
  return isProtectedAt(game, p.x, p.y);
}

function endLunge(game: MissionGame, finished: boolean): void {
  const snake = game.snake;
  if (snake.mode !== "lunge" && snake.mode !== "windup") return;
  snake.mode = "recover";
  snake.modeT = LUNGE_RECOVER;
  snake.lungeLeft = 0;
  snake.lungeCd = LUNGE_CD;
  if (finished) snake.anger = ANGER_TIME * 0.5;
}

function crashWall(game: MissionGame): void {
  const snake = game.snake;
  const extra = snake.body.length - MIN_LEN;
  if (extra >= 2) dropTail(game, snake.body.length - Math.max(2, Math.floor(extra / 2)));
  snake.mode = "recover";
  snake.modeT = 0.5;
  snake.grow = 0;
  snake.straight = TURN_LOCK;
  snake.phaseThrough = Math.max(snake.phaseThrough, 2.0);
  turnToOpen(game);
  game.shake = Math.max(game.shake, 4);
}

function crashBody(game: MissionGame, hit: number): void {
  const cutAt = hit < MIN_LEN ? Math.max(MIN_LEN, game.snake.body.length - 3) : hit;
  if (cutAt < game.snake.body.length && cutAt >= MIN_LEN) dropTail(game, cutAt);
  game.snake.mode = "recover";
  game.snake.modeT = 0.75;
  game.snake.grow = 0;
  game.snake.straight = TURN_LOCK;
  game.snake.phaseThrough = Math.max(game.snake.phaseThrough, 2.2);
  turnToOpen(game);
  game.shake = Math.max(game.shake, 6);
  game.score += 80;
}

function dropTail(game: MissionGame, cutAt: number): void {
  game.snake.body.splice(cutAt);
}

function turnToOpen(game: MissionGame): void {
  const head = game.snake.body[0];
  const target = huntFocus(game);
  // 先找真正安全的一格；穿身期间身体不算阻挡
  type Cand = { dir: Dir; dist: number; safe: boolean };
  const cands: Cand[] = [];
  for (const dir of ALL_DIRS) {
    const nx = head.x + dir.x;
    const ny = head.y + dir.y;
    if (nx < 0 || ny < 0 || nx >= COLS || ny >= ROWS) continue;
    const bodyHit = indexAt(game.snake, nx, ny, false) >= 0;
    const walkable = game.snake.phaseThrough > 0 || !bodyHit;
    if (!walkable) continue;
    const c = cellCenter(nx, ny);
    cands.push({
      dir,
      dist: Math.hypot(c.x - target.x, c.y - target.y),
      safe: !bodyHit,
    });
  }
  if (!cands.length) {
    // 极端情况：随便转一个不掉头的方向
    const opts = sideOptions(game.snake.dir);
    if (opts.length) game.snake.dir = opts[Math.floor(rand(game) * opts.length)];
    return;
  }
  // 优先空地，再在空地里离目标近；乱逛时反而选远一点逃离缠绕
  cands.sort((a, b) => {
    if (a.safe !== b.safe) return a.safe ? -1 : 1;
    return exposedPrey(game).length === 0 ? b.dist - a.dist : a.dist - b.dist;
  });
  game.snake.dir = cands[0].dir;
}

function sideOptions(dir: Dir): Dir[] {
  return ALL_DIRS.filter((d) => !(d.x === -dir.x && d.y === -dir.y));
}

function isSafe(_game: MissionGame, x: number, y: number): boolean {
  return x >= 0 && y >= 0 && x < COLS && y < ROWS;
}

function headCellClear(game: MissionGame): boolean {
  const head = game.snake.body[0];
  // 头所占格与身体其它节不重叠
  for (let i = 1; i < game.snake.body.length; i++) {
    if (game.snake.body[i].x === head.x && game.snake.body[i].y === head.y) return false;
  }
  return true;
}

function indexAt(snake: Snake, x: number, y: number, ignoreTail: boolean): number {
  const last = snake.body.length - (ignoreTail ? 1 : 0);
  for (let i = 0; i < last; i++) {
    if (snake.body[i].x === x && snake.body[i].y === y) return i;
  }
  return -1;
}

function checkSnakeKills(game: MissionGame): void {
  if (game.player.alive && headHitsMover(game, game.player) && !isProtectedAt(game, game.player.x, game.player.y)) {
    game.player.alive = false;
    game.phase = "dead";
    game.shake = 12;
    game.flash = 0.4;
    saveBest(game);
  }
  for (const bot of game.bots) {
    if (!bot.alive) continue;
    if (headHitsMover(game, bot) && !isProtectedAt(game, bot.x, bot.y)) {
      bot.alive = false;
      burst(game, { x: bot.x, y: bot.y }, bot.color, 8);
    }
  }
}

function headHitsMover(game: MissionGame, m: Mover): boolean {
  for (const snake of game.snakes) {
    const h = headPixelOf(snake);
    if (dist(m.x, m.y, h.x, h.y) < KILL_R) return true;
  }
  return false;
}

function nearestSnakeHead(game: MissionGame, x: number, y: number): { x: number; y: number } {
  let best = headPixelOf(game.snakes[0]);
  let bestD = dist(x, y, best.x, best.y);
  for (const snake of game.snakes) {
    const h = headPixelOf(snake);
    const d = dist(x, y, h.x, h.y);
    if (d < bestD) {
      bestD = d;
      best = h;
    }
  }
  return best;
}

function headPixel(game: MissionGame): { x: number; y: number } {
  return headPixelOf(game.snake);
}

function headPixelOf(snake: Snake): { x: number; y: number } {
  const head = snake.body[0];
  return cellCenter(head.x, head.y);
}

function snakeInterval(game: MissionGame): number {
  const k = clamp(game.elapsed / 50, 0, 1);
  let interval = 0.155 - 0.05 * k;
  if (game.snake.anger > 0) interval *= 0.75;
  return interval;
}

function updateCamera(game: MissionGame): void {
  const vw = game.view.viewportW;
  const vh = game.view.viewportH;
  const maxX = WORLD_W - vw;
  const maxY = WORLD_H - vh;
  if (maxX <= 0) game.camX = maxX / 2;
  else game.camX = clamp(game.player.x - vw / 2, 0, maxX);
  if (maxY <= 0) game.camY = maxY / 2;
  else game.camY = clamp(game.player.y - vh / 2, 0, maxY);
}

function decayFx(game: MissionGame, dt: number): void {
  game.shake = Math.max(0, game.shake - dt * 30);
  game.flash = Math.max(0, game.flash - dt * 1.4);
  for (const p of game.particles) {
    p.life -= dt;
    p.x += p.vx * dt;
    p.y += p.vy * dt;
  }
  game.particles = game.particles.filter((p) => p.life > 0);
  for (const f of game.floaters) {
    f.life -= dt;
    f.y -= 26 * dt;
  }
  game.floaters = game.floaters.filter((f) => f.life > 0);
}

function burst(
  game: MissionGame,
  at: { x: number; y: number },
  color: string,
  count: number,
): void {
  for (let i = 0; i < count; i++) {
    const a = rand(game) * Math.PI * 2;
    const s = 40 + rand(game) * 90;
    game.particles.push({
      x: at.x,
      y: at.y,
      vx: Math.cos(a) * s,
      vy: Math.sin(a) * s,
      life: 0.4,
      max: 0.5,
      color,
      r: 2 + rand(game) * 2,
    });
  }
}

function rand(game: MissionGame): number {
  game.seed = (Math.imul(game.seed, 1664525) + 1013904223) >>> 0;
  return game.seed / 4294967296;
}

function loadBest(game: MissionGame): void {
  try {
    const raw = localStorage.getItem(MISSION_SAVE_KEY);
    if (!raw) return;
    const data = JSON.parse(raw) as { bestScore?: number };
    if (typeof data.bestScore === "number") game.bestScore = data.bestScore;
  } catch {
    /* ignore */
  }
}

function saveBest(game: MissionGame): void {
  if (game.score > game.bestScore) game.bestScore = game.score;
  try {
    localStorage.setItem(MISSION_SAVE_KEY, JSON.stringify({ bestScore: game.bestScore }));
  } catch {
    /* ignore */
  }
}

export function worldToScreen(game: MissionGame, wx: number, wy: number): { x: number; y: number } {
  return { x: wx - game.camX + game.view.originX, y: wy - game.camY + game.view.originY };
}

export function formatTime(t: number): string {
  const m = Math.floor(t / 60);
  const s = Math.floor(t % 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export function finishRanking(game: MissionGame): { name: string; time: number; you: boolean }[] {
  const rows: { name: string; time: number; you: boolean }[] = [];
  if (game.player.finishTime >= 0) {
    rows.push({ name: "你", time: game.player.finishTime, you: true });
  }
  game.bots.forEach((b, i) => {
    if (b.finishTime >= 0) rows.push({ name: `机器人${i + 1}`, time: b.finishTime, you: false });
  });
  rows.sort((a, b) => a.time - b.time);
  return rows;
}
