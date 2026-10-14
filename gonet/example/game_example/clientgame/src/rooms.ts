import { Cmd, errText, parseBody, type Session } from "./session";

export const MODE_DUO = 4;

export interface RoomState {
  roomid: string;
  mode: number;
  capacity: number;
  seats: string[];
  phase: string;
}

export interface RoomNotice {
  kind: string;
  roomid: string;
  roleid: string;
  mode: number;
  capacity: number;
  seats: string[];
  phase: string;
}

export interface RoomZone {
  x: number;
  y: number;
  r: number;
  kind: string;
}

export interface RoomPlayer {
  roleid: string;
  x: number;
  y: number;
  alive: boolean;
}

export interface RoomEvent {
  kind: string;
  index: number;
  x: number;
  y: number;
  speed: number;
  until: number;
  reason: string;
}

export interface RoomBegin {
  roomid: string;
  mode: number;
  seed: number;
  seats: string[];
  zones: RoomZone[];
  players: RoomPlayer[];
  targetx: number;
  targety: number;
  index: number;
  until: number;
  speed: number;
  framehz: number;
  targets: number;
}

export interface RoomFrame {
  roomid: string;
  frame: number;
  ops: Array<{ ax: number; ay: number; dash: boolean }>;
  events: RoomEvent[];
  players: RoomPlayer[];
}

export interface RoomResult {
  roomid: string;
  win: boolean;
  reason: string;
  index: number;
  frame: number;
}

export function roomNoticeText(kind: string): string {
  if (kind === "join") return "有人加入了房间";
  if (kind === "leave") return "有人离开了房间";
  if (kind === "start") return "对局开始";
  if (kind === "settle") return "对局结束";
  return "房间有变化";
}

export async function createRoom(session: Session, mode: number, capacity: number): Promise<RoomState> {
  return readState(await session.request(Cmd.roomcreate, { mode, capacity }));
}

export async function joinRoom(session: Session, roomid: string): Promise<RoomState> {
  return readState(await session.request(Cmd.roomjoin, { roomid }));
}

export async function leaveRoom(session: Session): Promise<void> {
  await roomPlain(session, Cmd.roomleave);
}

export async function startRoom(session: Session): Promise<RoomState> {
  return readState(await session.request(Cmd.roomstart));
}

export async function settleRoom(session: Session): Promise<void> {
  await roomPlain(session, Cmd.roomsettle);
}

export function sendRoomOp(session: Session, ax: number, ay: number, dash: boolean): void {
  session.send(Cmd.roomop, { ax, ay, dash });
}

export function sendRoomDead(session: Session, frame: number): void {
  session.send(Cmd.roomdead, { frame });
}

export function parseRoomNotice(data: string): RoomNotice | null {
  const body = readObject(data);
  if (!body || typeof body.kind !== "string" || typeof body.roomid !== "string") return null;
  return {
    kind: body.kind,
    roomid: body.roomid,
    roleid: typeof body.roleid === "string" ? body.roleid : "",
    mode: num(body.mode),
    capacity: num(body.capacity),
    seats: strings(body.seats),
    phase: typeof body.phase === "string" ? body.phase : "",
  };
}

export function parseRoomBegin(data: string): RoomBegin | null {
  const body = readObject(data);
  if (!body || typeof body.roomid !== "string") return null;
  return {
    roomid: body.roomid,
    mode: num(body.mode),
    seed: num(body.seed) >>> 0,
    seats: strings(body.seats),
    zones: readZones(body.zones),
    players: readPlayers(body.players),
    targetx: num(body.targetx),
    targety: num(body.targety),
    index: num(body.index),
    until: num(body.until),
    speed: num(body.speed) || 1,
    framehz: num(body.framehz) || 20,
    targets: num(body.targets) || 10,
  };
}

export function parseRoomFrame(data: string): RoomFrame | null {
  const body = readObject(data);
  if (!body || typeof body.roomid !== "string") return null;
  return {
    roomid: body.roomid,
    frame: num(body.frame),
    ops: readOps(body.ops),
    events: readEvents(body.events),
    players: readPlayers(body.players),
  };
}

export function parseRoomResult(data: string): RoomResult | null {
  const body = readObject(data);
  if (!body || typeof body.roomid !== "string") return null;
  return {
    roomid: body.roomid,
    win: Boolean(body.win),
    reason: typeof body.reason === "string" ? body.reason : "",
    index: num(body.index),
    frame: num(body.frame),
  };
}

async function roomPlain(session: Session, cmd: number): Promise<void> {
  const body = parseBody(await session.request(cmd));
  if (body === "ok") return;
  const err = errText(body);
  throw new Error(err || "操作失败");
}

function readState(raw: string): RoomState {
  const body = parseBody(raw);
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object") throw new Error("房间回复异常");
  const card = body as Record<string, unknown>;
  if (typeof card.roomid !== "string" || card.roomid === "") throw new Error("房间回复异常");
  return {
    roomid: card.roomid,
    mode: num(card.mode),
    capacity: num(card.capacity),
    seats: strings(card.seats),
    phase: typeof card.phase === "string" ? card.phase : "wait",
  };
}

function readObject(data: string): Record<string, unknown> | null {
  try {
    const body = parseBody(data);
    if (!body || typeof body !== "object") return null;
    return body as Record<string, unknown>;
  } catch {
    return null;
  }
}

function num(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function strings(value: unknown): string[] {
  if (value == null) return [];
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string");
}

function readZones(value: unknown): RoomZone[] {
  if (!Array.isArray(value)) return [];
  const out: RoomZone[] = [];
  for (const item of value) {
    if (!item || typeof item !== "object") continue;
    const z = item as Record<string, unknown>;
    out.push({
      x: num(z.x),
      y: num(z.y),
      r: num(z.r),
      kind: typeof z.kind === "string" ? z.kind : "",
    });
  }
  return out;
}

function readPlayers(value: unknown): RoomPlayer[] {
  if (!Array.isArray(value)) return [];
  const out: RoomPlayer[] = [];
  for (const item of value) {
    if (!item || typeof item !== "object") continue;
    const p = item as Record<string, unknown>;
    out.push({
      roleid: typeof p.roleid === "string" ? p.roleid : "",
      x: num(p.x),
      y: num(p.y),
      alive: Boolean(p.alive),
    });
  }
  return out;
}

function readOps(value: unknown): Array<{ ax: number; ay: number; dash: boolean }> {
  if (!Array.isArray(value)) return [];
  return value.map((item) => {
    if (!item || typeof item !== "object") return { ax: 0, ay: 0, dash: false };
    const o = item as Record<string, unknown>;
    return { ax: num(o.ax), ay: num(o.ay), dash: Boolean(o.dash) };
  });
}

function readEvents(value: unknown): RoomEvent[] {
  if (!Array.isArray(value)) return [];
  const out: RoomEvent[] = [];
  for (const item of value) {
    if (!item || typeof item !== "object") continue;
    const e = item as Record<string, unknown>;
    out.push({
      kind: typeof e.kind === "string" ? e.kind : "",
      index: num(e.index),
      x: num(e.x),
      y: num(e.y),
      speed: num(e.speed),
      until: num(e.until),
      reason: typeof e.reason === "string" ? e.reason : "",
    });
  }
  return out;
}
