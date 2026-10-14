import {
  CELL,
  COLS,
  ROWS,
  WORLD_H,
  WORLD_W,
  cellCenter,
  clamp,
} from "./config";

export const MISSION_TIME = 180;
export const BOT_COUNT = 4;
export const BEACON_STAND = 1.0;
export const SAFE_SHELTER = 10;
export const SAFE_UNSTABLE = 4;
export const SAFE_COLLAPSE = 10;
export const SAFE_COOLDOWN = 12;
export const SAFE_RADIUS = 52;
export const BEACON_RADIUS = 28;
/** 玩法 3：当前目标点的限时（秒），超时失败。 */
export const ARENA_BEACON_TIME = 30;
/** 玩法 3：需要碰到的目标数量。 */
export const ARENA_TARGET_COUNT = 5;
/** 玩法 3：底部开始区半径。 */
export const START_RADIUS = 160;
/** 玩法 3：一处安全区、一处庇护区，都一直安全。 */
export const ARENA_SAFES: { x: number; y: number; mark: "safe" | "shelter" }[] = [
  { ...cellCenter(14, 10), mark: "safe" },
  { ...cellCenter(42, 14), mark: "shelter" },
];

export const MISSION_SAVE_KEY = "clientgame.mission.best";

/** 安全区候选中心（世界像素），避开边界。 */
export const SAFE_CANDIDATES: { x: number; y: number }[] = [
  cellCenter(10, 8),
  cellCenter(46, 8),
  cellCenter(10, 28),
  cellCenter(46, 28),
  cellCenter(28, 18),
  cellCenter(18, 18),
  cellCenter(38, 18),
  cellCenter(28, 8),
  cellCenter(28, 28),
];

/** 信标 A/B/C：C 远离常见安全区中心。 */
export function defaultBeacons(): { x: number; y: number; name: string }[] {
  return [
    { ...cellCenter(8, 18), name: "A" },
    { ...cellCenter(28, 6), name: "B" },
    { ...cellCenter(48, 30), name: "C" },
  ];
}

export function missionClamp(v: number, lo: number, hi: number): number {
  return clamp(v, lo, hi);
}

export function dist(ax: number, ay: number, bx: number, by: number): number {
  return Math.hypot(ax - bx, ay - by);
}

export { CELL, COLS, ROWS, WORLD_H, WORLD_W, cellCenter, clamp };
