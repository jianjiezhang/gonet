import { Cmd, errText, parseBody, type Session } from "./session";

export interface MissionRow {
  id: number;
  status: number;
  progress: number;
  target: number;
  reward: number;
}

export interface ClaimResult {
  id: number;
  reward: number;
  level: number;
  list: MissionRow[];
}

export function rewardLabel(reward: number): string {
  if (reward > 0) return `奖励 等级 +${reward}`;
  return "奖励 无";
}

export function statusLabel(status: number): string {
  if (status === 1) return "进行中";
  if (status === 2) return "可领取";
  if (status === 3) return "已领取";
  return `状态 ${status}`;
}

export async function loadMissions(session: Session): Promise<MissionRow[]> {
  const body = parseBody(await session.request(Cmd.missionlist));
  const err = errText(body);
  if (err) throw new Error(err);
  return readList(body);
}

export async function claimMission(session: Session, id: number): Promise<ClaimResult> {
  const body = parseBody(await session.request(Cmd.missionfinish, { id }));
  const err = errText(body);
  if (err) throw new Error(err);
  if (!body || typeof body !== "object") throw new Error("领取回复异常");
  const card = body as { id?: unknown; reward?: unknown; level?: unknown };
  if (typeof card.id !== "number" || typeof card.reward !== "number" || typeof card.level !== "number") {
    throw new Error("领取回复异常");
  }
  return { id: card.id, reward: card.reward, level: card.level, list: readList(body) };
}

function readList(body: unknown): MissionRow[] {
  if (!body || typeof body !== "object" || !("list" in body)) throw new Error("任务回复异常");
  const list = (body as { list: unknown }).list;
  if (list == null) return [];
  if (!Array.isArray(list)) throw new Error("任务回复异常");
  return list.map((item) => {
    if (!isRow(item)) throw new Error("任务回复异常");
    return item;
  });
}

function isRow(item: unknown): item is MissionRow {
  if (!item || typeof item !== "object") return false;
  const row = item as MissionRow;
  return (
    typeof row.id === "number" &&
    typeof row.status === "number" &&
    typeof row.progress === "number" &&
    typeof row.target === "number" &&
    typeof row.reward === "number"
  );
}
