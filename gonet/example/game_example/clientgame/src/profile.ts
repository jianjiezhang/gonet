import { Cmd, errText, parseBody, type Session } from "./session";

export interface RoleCard {
  roleid: string;
  level: number;
  name: string;
  gender: number;
}

export function genderLabel(gender: number): string {
  if (gender === 1) return "男";
  if (gender === 2) return "女";
  return "未设置";
}

export async function loadRole(session: Session): Promise<RoleCard> {
  const body = parseBody(await session.request(Cmd.roleinfo));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!isRoleCard(body)) throw new Error("资料回复异常");
  return body;
}

export async function saveLevel(session: Session, level: number): Promise<number> {
  const body = parseBody(await session.request(Cmd.setlevel, { level }));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object" || typeof (body as { level?: unknown }).level !== "number") {
    throw new Error("改等级回复异常");
  }
  return (body as { level: number }).level;
}

function isRoleCard(body: unknown): body is RoleCard {
  if (!body || typeof body !== "object") return false;
  const card = body as RoleCard;
  return typeof card.roleid === "string" && typeof card.name === "string" && typeof card.level === "number";
}
