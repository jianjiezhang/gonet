import { Cmd, errText, parseBody, type Session } from "./session";

export interface FriendPerson {
  roleid: string;
  name: string;
  level: number;
  online: boolean;
}

export interface FriendRequestRow extends FriendPerson {
  time: number;
}

export interface FriendList {
  friends: FriendPerson[];
  incoming: FriendRequestRow[];
  outgoing: FriendRequestRow[];
}

export function friendLine(person: FriendPerson, time = 0): string {
  const online = person.online ? "在线" : "离线";
  const base = `${person.name}　${person.roleid}　Lv.${person.level}　${online}`;
  if (!time) return base;
  const date = new Date(time * 1000);
  if (Number.isNaN(date.getTime())) return base;
  return `${base}　${date.toLocaleString()}`;
}

export function friendNoticeText(kind: string): string {
  if (kind === "apply") return "收到新的好友申请";
  if (kind === "agree") return "对方同意了好友申请";
  if (kind === "reject") return "对方拒绝了好友申请";
  if (kind === "delete") return "对方删除了好友";
  return "好友列表有变化";
}

export async function loadFriends(session: Session): Promise<FriendList> {
  const body = parseBody(await session.request(Cmd.friendlist));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object") throw new Error("好友回复异常");
  const card = body as { friends?: unknown; incoming?: unknown; outgoing?: unknown };
  return {
    friends: readPeople(card.friends),
    incoming: readRequests(card.incoming),
    outgoing: readRequests(card.outgoing),
  };
}

export async function applyFriend(session: Session, roleid: string): Promise<void> {
  await friendOp(session, Cmd.friendapply, roleid);
}

export async function agreeFriend(session: Session, roleid: string): Promise<void> {
  await friendOp(session, Cmd.friendagree, roleid);
}

export async function rejectFriend(session: Session, roleid: string): Promise<void> {
  await friendOp(session, Cmd.friendreject, roleid);
}

export async function deleteFriend(session: Session, roleid: string): Promise<void> {
  await friendOp(session, Cmd.frienddelete, roleid);
}

async function friendOp(session: Session, cmd: number, roleid: string): Promise<void> {
  const body = parseBody(await session.request(cmd, { roleid }));
  if (body === "ok") return;
  const err = errText(body);
  throw new Error(err || "操作失败");
}

function readPeople(value: unknown): FriendPerson[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new Error("好友回复异常");
  return value.map((item) => {
    if (!isPerson(item)) throw new Error("好友回复异常");
    return item;
  });
}

function readRequests(value: unknown): FriendRequestRow[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new Error("好友回复异常");
  return value.map((item) => {
    if (!isPerson(item)) throw new Error("好友回复异常");
    const time = (item as { time?: unknown }).time;
    return { ...item, time: typeof time === "number" ? time : 0 };
  });
}

function isPerson(item: unknown): item is FriendPerson {
  if (!item || typeof item !== "object") return false;
  const row = item as FriendPerson;
  return (
    typeof row.roleid === "string" &&
    typeof row.name === "string" &&
    typeof row.level === "number" &&
    typeof row.online === "boolean"
  );
}
