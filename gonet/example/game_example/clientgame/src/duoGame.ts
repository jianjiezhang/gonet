import {
  Chase,
  PLAYER_R_DUO,
  TARGET_RADIUS,
  type DuoPlayer,
  type DuoZone,
  type Head,
} from "./duoChase";
import type { RoomBegin, RoomFrame, RoomResult } from "./rooms";
import { wideView, type ViewLayout } from "./game";

export type DuoPlayPhase = "play" | "result";

export interface DuoMatch {
  roleID: string;
  roomid: string;
  view: ViewLayout;
  phase: DuoPlayPhase;
  seats: string[];
  zones: DuoZone[];
  players: DuoPlayer[];
  heads: Head[];
  bodies: Head[][];
  targetX: number;
  targetY: number;
  index: number;
  until: number;
  speed: number;
  frame: number;
  targets: number;
  chase: Chase;
  reported: boolean;
  result: RoomResult | null;
  flash: number;
}

export function createDuoMatch(roleID: string, begin: RoomBegin): DuoMatch {
  const zones = begin.zones.map((z) => ({ x: z.x, y: z.y, r: z.r, kind: z.kind }));
  const chase = new Chase(begin.seed, zones);
  return {
    roleID,
    roomid: begin.roomid,
    view: wideView(),
    phase: "play",
    seats: [...begin.seats],
    zones,
    players: begin.players.map(copyPlayer),
    heads: chase.heads(),
    bodies: chase.bodies(),
    targetX: begin.targetx,
    targetY: begin.targety,
    index: begin.index,
    until: begin.until,
    speed: begin.speed >= 1 ? begin.speed : 1,
    frame: 0,
    targets: begin.targets || 10,
    chase,
    reported: false,
    result: null,
    flash: 0,
  };
}

/** 吃下一帧。返回需要上报出局的帧号，没有则 0。 */
export function applyDuoFrame(match: DuoMatch, frame: RoomFrame): number {
  if (match.phase !== "play") return 0;
  match.frame = frame.frame;
  match.players = frame.players.map(copyPlayer);
  const dead = match.chase.step(match.players, match.speed);
  match.heads = match.chase.heads();
  match.bodies = match.chase.bodies();
  for (const ev of frame.events) {
    if (ev.kind === "pass" || ev.kind === "win") {
      if (ev.speed >= 1) match.speed = ev.speed;
      if (ev.kind === "pass") {
        match.index = ev.index;
        match.targetX = ev.x;
        match.targetY = ev.y;
        match.until = ev.until;
      }
    }
    if (ev.kind === "fail") {
      match.flash = 0.35;
    }
  }
  if (match.reported) return 0;
  if (dead.includes(match.roleID)) {
    match.reported = true;
    match.flash = 0.5;
    return frame.frame;
  }
  return 0;
}

export function applyDuoResult(match: DuoMatch, result: RoomResult): void {
  match.phase = "result";
  match.result = result;
  match.flash = 0.4;
}

export function tickDuoFx(match: DuoMatch, dt: number): void {
  if (match.flash > 0) match.flash = Math.max(0, match.flash - dt);
}

export function duoReasonText(reason: string): string {
  if (reason === "hit") return "目标全部完成";
  if (reason === "timeout") return "目标超时";
  if (reason === "bite") return "两人都被咬到";
  if (reason === "empty") return "场上无人";
  if (reason === "manual") return "手动结束";
  return reason || "结束";
}

export { PLAYER_R_DUO, TARGET_RADIUS };

function copyPlayer(p: { roleid: string; x: number; y: number; alive: boolean }): DuoPlayer {
  return { roleid: p.roleid, x: p.x, y: p.y, alive: p.alive };
}
