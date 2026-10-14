export const CELL = 32;

/** 世界格子：大约是旧图 3～4 倍面积。 */
export const COLS = 56;
export const ROWS = 36;

/** 屏幕上可见的格子（镜头跟随，地图比屏幕大）。 */
export const VIEW_COLS = 22;
export const VIEW_ROWS = 14;

export const HUD_H = 72;
export const FOOT_H = 32;
export const ORIGIN_X = 16;
export const ORIGIN_Y = HUD_H;

export const VIEWPORT_W = VIEW_COLS * CELL;
export const VIEWPORT_H = VIEW_ROWS * CELL;
export const WORLD_W = COLS * CELL;
export const WORLD_H = ROWS * CELL;

export const VIEW_W = ORIGIN_X * 2 + VIEWPORT_W;
export const VIEW_H = HUD_H + VIEWPORT_H + FOOT_H;

export const PLAYER_R = 11;
export const BODY_R = 13;
export const HEAD_R = 16;
export const KILL_R = PLAYER_R + 14;

export const MIN_LEN = 5;
export const MAX_LEN = 48;
export const START_LEN = 12;

export const PLAYER_SPEED = 272;
export const DASH_TIME = 0.14;
export const DASH_CD = 1.45;
export const DASH_MULT = 2.0;
export const BOOST_TIME = 2.2;
export const BOOST_MULT = 1.22;

/** 追击时至少直走几格才能转弯。1 = 有惯性但仍能贴身拐。 */
export const TURN_LOCK = 1;

/** 进入扑咬的接近距离（像素）。 */
export const LUNGE_RANGE = 268;
export const LUNGE_WINDUP = 0.3;
export const LUNGE_STEPS = 7;
export const LUNGE_CD = 1.35;
export const LUNGE_RECOVER = 0.36;
export const LUNGE_STEP_TIME = 0.072;
/** 躲开扑咬后，蛇短暂狂暴加速追击。 */
export const ANGER_TIME = 2.2;

export const MINIMAP_SIZE = 152;
export const MINIMAP_PAD = 10;

export const SAVE_KEY = "clientgame.reverseSnake.best";

export interface Dir {
  x: number;
  y: number;
}

/** 世界坐标：格子中心。 */
export function cellCenter(x: number, y: number): { x: number; y: number } {
  return {
    x: x * CELL + CELL / 2,
    y: y * CELL + CELL / 2,
  };
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}
