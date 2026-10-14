import { Cmd, errText, parseBody, type Session } from "./session";

export interface GuildBrief {
  guildid: string;
  name: string;
  notice: string;
  leader: string;
  leadername: string;
  level: number;
  online: boolean;
  members: number;
}

export interface GuildMember {
  roleid: string;
  name: string;
  level: number;
  online: boolean;
  rank: number;
  time: number;
}

export interface GuildApply {
  roleid: string;
  name: string;
  level: number;
  online: boolean;
  time: number;
}

export interface MyGuild {
  guildid: string;
  name: string;
  notice: string;
  leader: string;
  members: GuildMember[];
  applies: GuildApply[];
}

export function rankLabel(rank: number): string {
  if (rank === 1) return "会长";
  if (rank === 2) return "成员";
  return `职位 ${rank}`;
}

export function memberLine(person: GuildMember | GuildApply, rank?: number): string {
  const online = person.online ? "在线" : "离线";
  const base = `${person.name}　${person.roleid}　Lv.${person.level}　${online}`;
  const title = rank === undefined ? base : `${base}　${rankLabel(rank)}`;
  if (!person.time) return title;
  const date = new Date(person.time * 1000);
  if (Number.isNaN(date.getTime())) return title;
  return `${title}　${date.toLocaleString()}`;
}

export function catalogLine(row: GuildBrief): string {
  const online = row.online ? "在线" : "离线";
  const notice = row.notice ? `　${row.notice}` : "";
  return `${row.name}　${row.members}人　会长 ${row.leadername}　Lv.${row.level}　${online}${notice}`;
}

export function guildNoticeText(kind: string): string {
  if (kind === "guild.apply") return "有人申请加入公会";
  if (kind === "guild.agree") return "你已加入公会";
  if (kind === "guild.reject") return "入会申请被拒绝";
  if (kind === "guild.leave") return "有成员退出公会";
  if (kind === "guild.kick") return "你被移出公会";
  if (kind === "guild.disband") return "公会已解散";
  return "公会有变化";
}

export async function findGuildByID(session: Session, guildid: string): Promise<GuildBrief> {
  return findGuild(session, Cmd.guildid, { guildid });
}

export async function findGuildByName(session: Session, name: string): Promise<GuildBrief> {
  return findGuild(session, Cmd.guildname, { name });
}

async function findGuild(session: Session, cmd: number, body: unknown): Promise<GuildBrief> {
  const reply = parseBody(await session.request(cmd, body));
  const err = errText(reply);
  if (err) throw new Error(err);
  if (!isBrief(reply)) throw new Error("公会回复异常");
  return reply;
}

export async function loadGuildCatalog(session: Session): Promise<GuildBrief[]> {
  const body = parseBody(await session.request(Cmd.guilds));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object" || !("guilds" in body)) throw new Error("公会回复异常");
  return readBriefs((body as { guilds: unknown }).guilds);
}

export async function loadMyGuild(session: Session): Promise<MyGuild | null> {
  const body = parseBody(await session.request(Cmd.guildlist));
  const err = errText(body);
  if (err === "还没有公会") return null;
  if (err) throw new Error(err);
  if (!isMyGuild(body)) throw new Error("公会回复异常");
  return {
    guildid: body.guildid,
    name: body.name,
    notice: body.notice,
    leader: body.leader,
    members: readMembers(body.members),
    applies: readApplies(body.applies),
  };
}

export async function createGuild(session: Session, name: string, notice: string): Promise<{ guildid: string; name: string }> {
  const body = parseBody(await session.request(Cmd.guildcreate, { name, notice }));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object") throw new Error("创建回复异常");
  const card = body as { guildid?: unknown; name?: unknown };
  if (typeof card.guildid !== "string" || typeof card.name !== "string") throw new Error("创建回复异常");
  return { guildid: card.guildid, name: card.name };
}

export async function applyGuild(session: Session, guildid: string): Promise<void> {
  await guildAck(session, Cmd.guildapply, { guildid });
}

export async function agreeGuild(session: Session, roleid: string): Promise<void> {
  await guildAck(session, Cmd.guildagree, { roleid });
}

export async function rejectGuild(session: Session, roleid: string): Promise<void> {
  await guildAck(session, Cmd.guildreject, { roleid });
}

export async function kickGuild(session: Session, roleid: string): Promise<void> {
  await guildAck(session, Cmd.guildkick, { roleid });
}

export async function leaveGuild(session: Session): Promise<void> {
  await guildAck(session, Cmd.guildleave);
}

export async function disbandGuild(session: Session): Promise<void> {
  await guildAck(session, Cmd.guilddisband);
}

async function guildAck(session: Session, cmd: number, body?: unknown): Promise<void> {
  const reply = parseBody(await session.request(cmd, body));
  if (reply === "ok") return;
  const err = errText(reply);
  throw new Error(err || "操作失败");
}

function readBriefs(value: unknown): GuildBrief[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new Error("公会回复异常");
  return value.map((item) => {
    if (!isBrief(item)) throw new Error("公会回复异常");
    return item;
  });
}

function readMembers(value: unknown): GuildMember[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new Error("公会回复异常");
  return value.map((item) => {
    if (!isMember(item)) throw new Error("公会回复异常");
    return item;
  });
}

function readApplies(value: unknown): GuildApply[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new Error("公会回复异常");
  return value.map((item) => {
    if (!isApply(item)) throw new Error("公会回复异常");
    return item;
  });
}

function isBrief(item: unknown): item is GuildBrief {
  if (!item || typeof item !== "object") return false;
  const row = item as GuildBrief;
  return (
    typeof row.guildid === "string" &&
    typeof row.name === "string" &&
    typeof row.notice === "string" &&
    typeof row.leader === "string" &&
    typeof row.leadername === "string" &&
    typeof row.level === "number" &&
    typeof row.online === "boolean" &&
    typeof row.members === "number"
  );
}

function isMyGuild(body: unknown): body is MyGuild {
  if (!body || typeof body !== "object") return false;
  const row = body as MyGuild;
  return (
    typeof row.guildid === "string" &&
    typeof row.name === "string" &&
    typeof row.notice === "string" &&
    typeof row.leader === "string"
  );
}

function isMember(item: unknown): item is GuildMember {
  if (!isApply(item)) return false;
  return typeof (item as GuildMember).rank === "number";
}

function isApply(item: unknown): item is GuildApply {
  if (!item || typeof item !== "object") return false;
  const row = item as GuildApply;
  return (
    typeof row.roleid === "string" &&
    typeof row.name === "string" &&
    typeof row.level === "number" &&
    typeof row.online === "boolean" &&
    typeof row.time === "number"
  );
}
