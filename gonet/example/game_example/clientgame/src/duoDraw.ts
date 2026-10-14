import { BODY_R, COLS, HEAD_R, PLAYER_R, ROWS, WORLD_H, WORLD_W, cellCenter } from "./config";
import { PLAYER_R_DUO, TARGET_RADIUS, ZONE_SAFE, ZONE_START } from "./duoChase";
import { duoReasonText, type DuoMatch } from "./duoGame";

export function drawDuo(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  const { screenW, screenH, originX, originY, viewportW, viewportH } = match.view;
  ctx.clearRect(0, 0, screenW, screenH);
  ctx.fillStyle = "#0b0e13";
  ctx.fillRect(0, 0, screenW, screenH);

  ctx.save();
  ctx.beginPath();
  ctx.rect(originX, originY, viewportW, viewportH);
  ctx.clip();

  ctx.fillStyle = "#141a24";
  ctx.fillRect(originX, originY, viewportW, viewportH);
  drawDots(ctx, match);
  drawBorder(ctx, match);
  drawZones(ctx, match);
  drawTarget(ctx, match);
  for (const body of match.bodies) drawSnake(ctx, match, body);
  for (const p of match.players) drawPlayer(ctx, match, p);

  ctx.restore();

  drawHud(ctx, match);
  if (match.flash > 0) {
    ctx.fillStyle = `rgba(255,64,64,${match.flash * 0.4})`;
    ctx.fillRect(0, 0, screenW, screenH);
  }
  if (match.phase === "result" && match.result) {
    const title = match.result.win ? "任务完成" : "任务失败";
    const color = match.result.win ? "#b6ff9a" : "#ffd0d0";
    drawEnd(ctx, match, title, duoReasonText(match.result.reason), color);
  }
}

function toScreen(match: DuoMatch, wx: number, wy: number): { x: number; y: number } {
  return { x: wx + match.view.originX, y: wy + match.view.originY };
}

function drawDots(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  ctx.fillStyle = "rgba(255,255,255,0.035)";
  for (let y = 0; y < ROWS; y++) {
    for (let x = 0; x < COLS; x++) {
      const c = cellCenter(x, y);
      const s = toScreen(match, c.x, c.y);
      ctx.fillRect(s.x - 1, s.y - 1, 2, 2);
    }
  }
}

function drawBorder(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  const tl = toScreen(match, 0, 0);
  const br = toScreen(match, WORLD_W, WORLD_H);
  ctx.strokeStyle = "#5a6f8c";
  ctx.lineWidth = 4;
  ctx.strokeRect(tl.x, tl.y, br.x - tl.x, br.y - tl.y);
}

function drawZones(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  for (const z of match.zones) {
    const s = toScreen(match, z.x, z.y);
    const style = zoneStyle(z.kind);
    ctx.beginPath();
    ctx.arc(s.x, s.y, z.r, 0, Math.PI * 2);
    ctx.fillStyle = style.fill;
    ctx.fill();
    ctx.strokeStyle = style.stroke;
    ctx.lineWidth = 2;
    ctx.stroke();
    ctx.fillStyle = style.text;
    ctx.font = "12px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(style.label, s.x, s.y + 4);
  }
}

function zoneStyle(kind: string): { fill: string; stroke: string; text: string; label: string } {
  if (kind === ZONE_START) {
    return { fill: "rgba(70, 200, 120, 0.22)", stroke: "#7dffb0", text: "#e7fff0", label: "出发" };
  }
  if (kind === ZONE_SAFE) {
    return { fill: "rgba(70, 200, 120, 0.22)", stroke: "#7dffb0", text: "#e7fff0", label: "安全" };
  }
  return { fill: "rgba(255,255,255,0.08)", stroke: "#9aa8bd", text: "#e8eef7", label: kind };
}

function drawTarget(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  if (match.phase === "result") return;
  const s = toScreen(match, match.targetX, match.targetY);
  ctx.beginPath();
  ctx.arc(s.x, s.y, TARGET_RADIUS, 0, Math.PI * 2);
  ctx.fillStyle = "rgba(255, 210, 80, 0.28)";
  ctx.fill();
  ctx.strokeStyle = "#ffd15c";
  ctx.lineWidth = 3;
  ctx.stroke();
  ctx.fillStyle = "#ffe9a8";
  ctx.font = "700 14px sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(String(match.index + 1), s.x, s.y + 5);
}

function drawSnake(ctx: CanvasRenderingContext2D, match: DuoMatch, body: Array<{ x: number; y: number }>): void {
  if (!body.length) return;
  ctx.lineCap = "round";
  ctx.strokeStyle = "#0d3d28";
  ctx.lineWidth = BODY_R * 2.1;
  ctx.beginPath();
  body.forEach((seg, i) => {
    const s = toScreen(match, seg.x, seg.y);
    if (i === 0) ctx.moveTo(s.x, s.y);
    else ctx.lineTo(s.x, s.y);
  });
  ctx.stroke();
  for (let i = body.length - 1; i >= 1; i--) {
    const s = toScreen(match, body[i].x, body[i].y);
    const t = i / body.length;
    ctx.fillStyle = t > 0.5 ? "#145a38" : "#1f8f4e";
    ctx.beginPath();
    ctx.arc(s.x, s.y, BODY_R * (1 - t * 0.3), 0, Math.PI * 2);
    ctx.fill();
  }
  const head = body[0];
  const prev = body[1] ?? head;
  let dx = head.x - prev.x;
  let dy = head.y - prev.y;
  const len = Math.hypot(dx, dy);
  if (len < 0.001) {
    dx = 1;
    dy = 0;
  } else {
    dx /= len;
    dy /= len;
  }
  const hs = toScreen(match, head.x, head.y);
  const tip = HEAD_R + 2;
  const px = -dy;
  const py = dx;
  ctx.beginPath();
  ctx.moveTo(hs.x + dx * tip, hs.y + dy * tip);
  ctx.lineTo(hs.x - dx * HEAD_R * 0.85 + px * HEAD_R, hs.y - dy * HEAD_R * 0.85 + py * HEAD_R);
  ctx.lineTo(hs.x - dx * HEAD_R * 0.85 - px * HEAD_R, hs.y - dy * HEAD_R * 0.85 - py * HEAD_R);
  ctx.closePath();
  ctx.fillStyle = "#e01222";
  ctx.fill();
}

function drawPlayer(
  ctx: CanvasRenderingContext2D,
  match: DuoMatch,
  p: { roleid: string; x: number; y: number; alive: boolean },
): void {
  const s = toScreen(match, p.x, p.y);
  const you = p.roleid === match.roleID;
  const r = PLAYER_R_DUO || PLAYER_R;
  ctx.beginPath();
  ctx.arc(s.x, s.y, r, 0, Math.PI * 2);
  if (!p.alive) {
    ctx.fillStyle = "rgba(150,150,160,0.45)";
    ctx.fill();
    ctx.strokeStyle = "#7a7f8c";
  } else {
    ctx.fillStyle = you ? "#7ad7ff" : "#b6ff9a";
    ctx.fill();
    ctx.strokeStyle = you ? "#e8fbff" : "#d8ffe8";
  }
  ctx.lineWidth = 2;
  ctx.stroke();
  ctx.fillStyle = "#e8eef7";
  ctx.font = "11px sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(you ? "你" : p.roleid.slice(-4), s.x, s.y - r - 8);
}

function drawHud(ctx: CanvasRenderingContext2D, match: DuoMatch): void {
  const left = Math.max(0, match.until - match.frame);
  const sec = (left / 20).toFixed(1);
  ctx.fillStyle = "rgba(11,14,19,0.72)";
  ctx.fillRect(12, 12, 320, 64);
  ctx.fillStyle = "#f4f7fb";
  ctx.font = "600 16px sans-serif";
  ctx.textAlign = "left";
  ctx.fillText(`目标 ${Math.min(match.index + 1, match.targets)}/${match.targets}`, 24, 36);
  ctx.fillStyle = "#9aa8bd";
  ctx.font = "13px sans-serif";
  ctx.fillText(`限时 ${sec}s · 蛇速 ×${match.speed.toFixed(2)} · 帧 ${match.frame}`, 24, 58);
  ctx.fillStyle = "#7f91a8";
  ctx.font = "12px sans-serif";
  ctx.textAlign = "right";
  ctx.fillText("WASD 移动 · 空格冲刺 · Esc 离开", match.view.screenW - 16, 28);
}

function drawEnd(ctx: CanvasRenderingContext2D, match: DuoMatch, title: string, sub: string, color: string): void {
  const { screenW, screenH } = match.view;
  ctx.fillStyle = "rgba(0,0,0,0.55)";
  ctx.fillRect(0, 0, screenW, screenH);
  ctx.fillStyle = color;
  ctx.font = "700 42px sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(title, screenW / 2, screenH / 2 - 12);
  ctx.fillStyle = "#e8eef7";
  ctx.font = "18px sans-serif";
  ctx.fillText(sub, screenW / 2, screenH / 2 + 28);
  ctx.fillStyle = "#9aa8bd";
  ctx.font = "14px sans-serif";
  ctx.fillText("Esc / 返回 离开", screenW / 2, screenH / 2 + 60);
}
