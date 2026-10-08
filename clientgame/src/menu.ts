import {
  FOOT_H,
  ORIGIN_X,
  ORIGIN_Y,
  VIEWPORT_H,
  VIEWPORT_W,
  VIEW_H,
  VIEW_W,
} from "./config";

export type AppMode = "menu" | "chase" | "mission" | "arena";
export type MenuPick = "chase" | "mission" | "arena";

export interface MenuState {
  selected: 0 | 1 | 2;
}

const OPTIONS: Array<{ pick: MenuPick; title: string; desc: string }> = [
  { pick: "chase", title: "玩法 1 · 猎人追逐", desc: "单人生存，躲开扑咬，诱蛇断身" },
  { pick: "mission", title: "玩法 2 · 猎场信使", desc: "限时 A→B→C，安全区会变，4 个机器人同场" },
  { pick: "arena", title: "玩法 3 · 全图信使", desc: "底部出发，一次一个目标，碰满 5 个" },
];

export function createMenu(): MenuState {
  return { selected: 0 };
}

export function updateMenu(
  menu: MenuState,
  input: { ax: number; ay: number; enterEdge: boolean; clickEdge: boolean; spaceEdge: boolean },
): MenuPick | null {
  if (input.ay < 0) menu.selected = ((menu.selected + 2) % 3) as 0 | 1 | 2;
  else if (input.ay > 0) menu.selected = ((menu.selected + 1) % 3) as 0 | 1 | 2;
  else if (input.ax < 0) menu.selected = ((menu.selected + 2) % 3) as 0 | 1 | 2;
  else if (input.ax > 0) menu.selected = ((menu.selected + 1) % 3) as 0 | 1 | 2;
  if (input.enterEdge || input.spaceEdge || input.clickEdge) {
    return OPTIONS[menu.selected].pick;
  }
  return null;
}

export function pickMenuAt(y: number): 0 | 1 | 2 {
  const top = ORIGIN_Y + 168;
  const step = 78;
  if (y < top + step) return 0;
  if (y < top + step * 2) return 1;
  return 2;
}

export function drawMenu(ctx: CanvasRenderingContext2D, menu: MenuState, roleID = ""): void {
  ctx.clearRect(0, 0, VIEW_W, VIEW_H);
  ctx.fillStyle = "#0b0e13";
  ctx.fillRect(0, 0, VIEW_W, VIEW_H);

  ctx.fillStyle = "#141a24";
  ctx.fillRect(ORIGIN_X, ORIGIN_Y, VIEWPORT_W, VIEWPORT_H);
  ctx.strokeStyle = "#46556d";
  ctx.lineWidth = 3;
  ctx.strokeRect(ORIGIN_X + 1.5, ORIGIN_Y + 1.5, VIEWPORT_W - 3, VIEWPORT_H - 3);

  ctx.textAlign = "center";
  ctx.fillStyle = "#f4f7fb";
  ctx.font = "700 34px sans-serif";
  ctx.fillText("反向贪吃蛇", VIEW_W / 2, ORIGIN_Y + 88);

  ctx.font = "15px sans-serif";
  ctx.fillStyle = "#9aa8bd";
  ctx.fillText(roleID ? `已登录 ${roleID} · 对局在本机` : "选择玩法", VIEW_W / 2, ORIGIN_Y + 126);

  OPTIONS.forEach((opt, i) => {
    drawOption(ctx, VIEW_W / 2, ORIGIN_Y + 200 + i * 78, opt.title, opt.desc, menu.selected === i);
  });

  ctx.fillStyle = "#7f91a8";
  ctx.font = "13px sans-serif";
  ctx.fillText("↑↓ / WASD 选择    Enter / 点击 开始", VIEW_W / 2, VIEW_H - FOOT_H / 2);
}

function drawOption(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  title: string,
  desc: string,
  on: boolean,
): void {
  const w = 460;
  const h = 66;
  const x = cx - w / 2;
  const y = cy - h / 2;
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, 12);
  ctx.fillStyle = on ? "rgba(40, 70, 90, 0.95)" : "rgba(22, 28, 38, 0.9)";
  ctx.fill();
  ctx.strokeStyle = on ? "#7ad7ff" : "#3e4d63";
  ctx.lineWidth = on ? 2.5 : 1.5;
  ctx.stroke();
  ctx.fillStyle = on ? "#e8fbff" : "#e8eef7";
  ctx.font = "600 17px sans-serif";
  ctx.fillText(title, cx, cy - 8);
  ctx.fillStyle = "#9aa8bd";
  ctx.font = "12px sans-serif";
  ctx.fillText(desc, cx, cy + 14);
}
