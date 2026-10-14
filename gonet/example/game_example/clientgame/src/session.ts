// 命令号与 protocol/client/cmd.go 一致。
// 心跳由游戏进程写到玩家口，页面不再另发。
// friendnotify、guildnotify 只由服务器推送，用 on 收，不要用 request。
export const Cmd = {
  login: 1,
  kick: 2,
  missionlist: 6,
  missionfinish: 7,
  roleinfo: 8,
  setlevel: 9,
  friendlist: 10,
  friendapply: 11,
  friendagree: 12,
  friendreject: 13,
  frienddelete: 14,
  friendnotify: 15,
  guildcreate: 16,
  guildlist: 17,
  guildapply: 18,
  guildagree: 19,
  guildreject: 20,
  guildleave: 21,
  guildkick: 22,
  guilddisband: 23,
  guildnotify: 24,
  guilds: 25,
  guildid: 26,
  guildname: 27,
  roomcreate: 28,
  roomjoin: 29,
  roomleave: 30,
  roomstart: 31,
  roomsettle: 32,
  roomnotify: 33,
  roomop: 34,
  roombegin: 35,
  roomframe: 36,
  roomresult: 37,
  roomdead: 38,
} as const;

export interface FriendNotice {
  kind: string;
  roleid: string;
  name: string;
  time: number;
}

export interface GuildNotice {
  kind: string;
  guildid: string;
  roleid: string;
  name: string;
  time: number;
}

type Waiter = {
  resolve: (data: string) => void;
  reject: (err: Error) => void;
};

export class Session {
  onDrop: (() => void) | null = null;
  private readonly ws: WebSocket;
  private readonly waiters = new Map<number, Waiter>();
  private readonly listeners = new Map<number, Array<(data: string) => void>>();
  private dropped = false;

  constructor(ws: WebSocket, onClose?: () => void) {
    this.ws = ws;
    ws.onmessage = (event) => {
      if (!(event.data instanceof ArrayBuffer)) return;
      let msg: { cmd: number; data: string };
      try {
        msg = decodeFrame(event.data);
      } catch {
        this.close();
        return;
      }
      const waiter = this.waiters.get(msg.cmd);
      if (waiter) {
        this.waiters.delete(msg.cmd);
        waiter.resolve(msg.data);
      }
      for (const fn of this.listeners.get(msg.cmd) ?? []) fn(msg.data);
    };
    ws.onclose = () => {
      this.drop();
      onClose?.();
    };
  }

  send(cmd: number, body?: unknown): void {
    if (this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(encodeFrame(cmd, body));
  }

  request(cmd: number, body?: unknown): Promise<string> {
    let timer = 0;
    const pending = new Promise<string>((resolve, reject) => {
      const waiter: Waiter = {
        resolve: (data) => {
          window.clearTimeout(timer);
          resolve(data);
        },
        reject: (err) => {
          window.clearTimeout(timer);
          reject(err);
        },
      };
      timer = window.setTimeout(() => {
        if (this.waiters.get(cmd) !== waiter) return;
        this.waiters.delete(cmd);
        reject(new Error("没有回复"));
      }, 8000);
      this.waiters.set(cmd, waiter);
    });
    this.send(cmd, body);
    return pending;
  }

  on(cmd: number, fn: (data: string) => void): void {
    const list = this.listeners.get(cmd) ?? [];
    list.push(fn);
    this.listeners.set(cmd, list);
  }

  close(): void {
    this.dropped = true;
    this.rejectWaiters();
    this.ws.close();
  }

  private drop(): void {
    if (this.dropped) return;
    this.dropped = true;
    this.rejectWaiters();
    this.onDrop?.();
  }

  private rejectWaiters(): void {
    for (const waiter of this.waiters.values()) waiter.reject(new Error("连接已断开"));
    this.waiters.clear();
  }
}

export function openSession(): Promise<Session> {
  return new Promise((resolve, reject) => {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${location.host}/api/session`);
    ws.binaryType = "arraybuffer";
    let settled = false;
    let session: Session;
    const finish = (err?: Error) => {
      if (settled) return;
      settled = true;
      if (err) reject(err);
      else resolve(session);
    };
    session = new Session(ws, () => finish(new Error("连不上游戏服")));
    ws.onopen = () => finish();
    ws.onerror = () => finish(new Error("连不上游戏服"));
  });
}

export function parseBody(data: string): unknown {
  if (data === "") return null;
  return JSON.parse(data) as unknown;
}

export function errText(body: unknown): string {
  if (!body || typeof body !== "object" || !("err" in body)) return "";
  const err = String((body as { err: unknown }).err);
  if (err === "auth") return "口令不对";
  return err;
}

export function parseFriendNotice(data: string): FriendNotice | null {
  const body = readNotice(data);
  if (!body || typeof body.kind !== "string" || typeof body.roleid !== "string" || body.roleid === "") return null;
  return {
    kind: body.kind,
    roleid: body.roleid,
    name: typeof body.name === "string" ? body.name : "",
    time: typeof body.time === "number" ? body.time : 0,
  };
}

export function parseGuildNotice(data: string): GuildNotice | null {
  const body = readNotice(data);
  if (!body || typeof body.kind !== "string" || body.kind === "") return null;
  return {
    kind: body.kind,
    guildid: typeof body.guildid === "string" ? body.guildid : "",
    roleid: typeof body.roleid === "string" ? body.roleid : "",
    name: typeof body.name === "string" ? body.name : "",
    time: typeof body.time === "number" ? body.time : 0,
  };
}

function readNotice(data: string): Record<string, unknown> | null {
  try {
    const body = parseBody(data);
    if (!body || typeof body !== "object") return null;
    return body as Record<string, unknown>;
  } catch {
    return null;
  }
}

function encodeFrame(cmd: number, body: unknown): ArrayBuffer {
  const data = body === undefined ? new Uint8Array(0) : new TextEncoder().encode(JSON.stringify(body));
  const n = 2 + data.length;
  const buf = new ArrayBuffer(4 + n);
  const view = new DataView(buf);
  view.setUint32(0, n);
  view.setUint16(4, cmd);
  new Uint8Array(buf, 6).set(data);
  return buf;
}

function decodeFrame(buf: ArrayBuffer): { cmd: number; data: string } {
  if (buf.byteLength < 6) throw new Error("坏帧");
  const view = new DataView(buf);
  const n = view.getUint32(0);
  const cmd = view.getUint16(4);
  if (n < 2 || 4 + n !== buf.byteLength || cmd === 0) throw new Error("坏帧");
  const data = new TextDecoder().decode(new Uint8Array(buf, 6, n - 2));
  return { cmd, data };
}
