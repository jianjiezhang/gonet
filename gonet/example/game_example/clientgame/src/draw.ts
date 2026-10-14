import {
  BODY_R,
  CELL,
  COLS,
  FOOT_H,
  HEAD_R,
  LUNGE_STEPS,
  MINIMAP_PAD,
  MINIMAP_SIZE,
  PLAYER_R,
  ROWS,
  WORLD_H,
  WORLD_W,
  cellCenter,
} from "./config";
import { formatTime, worldToScreen, type Game } from "./game";

export function draw(ctx: CanvasRenderingContext2D, game: Game): void {
  const { screenW, screenH, originX, originY, viewportW, viewportH, wide } = game.view;
  ctx.clearRect(0, 0, screenW, screenH);
  ctx.fillStyle = "#0b0e13";
  ctx.fillRect(0, 0, screenW, screenH);

  ctx.save();
  if (game.shake > 0) {
    ctx.translate((Math.random() - 0.5) * game.shake, (Math.random() - 0.5) * game.shake);
  }

  ctx.beginPath();
  ctx.rect(originX, originY, viewportW, viewportH);
  ctx.clip();

  drawArena(ctx, game);
  drawFruits(ctx, game);
  drawLungeTelegraph(ctx, game);
  drawSnake(ctx, game);
  drawPlayer(ctx, game);
  drawFx(ctx, game);
  ctx.restore();

  if (!wide) {
    ctx.strokeStyle = "#46556d";
    ctx.lineWidth = 3;
    ctx.strokeRect(originX + 1.5, originY + 1.5, viewportW - 3, viewportH - 3);
  }

  drawHud(ctx, game);
  drawMinimap(ctx, game);

  if (game.flash > 0) {
    ctx.fillStyle = `rgba(255, 64, 64, ${game.flash * 0.4})`;
    ctx.fillRect(0, 0, screenW, screenH);
  }
  if (game.phase === "title") drawTitle(ctx, game);
  if (game.phase === "dead") drawDead(ctx, game);
}

function drawArena(ctx: CanvasRenderingContext2D, game: Game): void {
  const { originX, originY, viewportW, viewportH } = game.view;
  ctx.fillStyle = "#141a24";
  ctx.fillRect(originX, originY, viewportW, viewportH);

  const x0 = Math.floor(game.camX / CELL);
  const y0 = Math.floor(game.camY / CELL);
  const x1 = Math.min(COLS, x0 + Math.ceil(viewportW / CELL) + 2);
  const y1 = Math.min(ROWS, y0 + Math.ceil(viewportH / CELL) + 2);

  ctx.fillStyle = "rgba(255,255,255,0.035)";
  for (let y = Math.max(0, y0); y <= y1; y++) {
    for (let x = Math.max(0, x0); x <= x1; x++) {
      const c = cellCenter(x, y);
      const s = worldToScreen(game, c.x, c.y);
      ctx.fillRect(s.x - 1, s.y - 1, 2, 2);
    }
  }

  const tl = worldToScreen(game, 0, 0);
  const br = worldToScreen(game, WORLD_W, WORLD_H);
  ctx.strokeStyle = "#2a3548";
  ctx.lineWidth = 4;
  ctx.strokeRect(tl.x, tl.y, br.x - tl.x, br.y - tl.y);
}

function drawLungeTelegraph(ctx: CanvasRenderingContext2D, game: Game): void {
  const snake = game.snake;
  if (snake.mode !== "windup" && snake.mode !== "lunge") return;
  const head = snake.body[0];
  if (!head) return;
  const dir = snake.dir;
  const steps = snake.mode === "lunge" ? snake.lungeLeft : LUNGE_STEPS;
  const pulse = 0.35 + 0.25 * Math.sin(game.time * 18);
  for (let i = 1; i <= steps; i++) {
    const x = head.x + dir.x * i;
    const y = head.y + dir.y * i;
    if (x < 0 || y < 0 || x >= COLS || y >= ROWS) break;
    const c = cellCenter(x, y);
    const s = worldToScreen(game, c.x, c.y);
    if (!inView(game, s.x, s.y, 20)) continue;
    ctx.fillStyle = snake.mode === "windup" ? `rgba(255, 140, 40, ${pulse})` : `rgba(255, 50, 60, ${0.45 + pulse * 0.3})`;
    ctx.fillRect(s.x - CELL / 2 + 2, s.y - CELL / 2 + 2, CELL - 4, CELL - 4);
  }
}

function drawFruits(ctx: CanvasRenderingContext2D, game: Game): void {
  for (const fruit of game.fruits) {
    const c = cellCenter(fruit.x, fruit.y);
    const s = worldToScreen(game, c.x, c.y);
    if (!inView(game, s.x, s.y, 20)) continue;
    const blink = fruit.ttl > 0 && fruit.ttl < 2.2 ? Math.floor(game.time * 8) % 2 === 0 : true;
    if (!blink) continue;
    ctx.fillStyle = "#ffd15c";
    ctx.beginPath();
    ctx.arc(s.x, s.y, 7, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = "#fff3b0";
    ctx.beginPath();
    ctx.arc(s.x - 2, s.y - 2, 2.5, 0, Math.PI * 2);
    ctx.fill();
  }
}

function drawSnake(ctx: CanvasRenderingContext2D, game: Game): void {
  const body = game.snake.body;
  if (body.length === 0) return;
  for (let i = body.length - 1; i >= 1; i--) {
    const c = cellCenter(body[i].x, body[i].y);
    const s = worldToScreen(game, c.x, c.y);
    if (!inView(game, s.x, s.y, 30)) continue;
    const t = i / body.length;
    ctx.fillStyle = game.snake.mode === "recover" ? `rgb(${90 + t * 40},${110 + t * 20},${110})` : `rgb(${30 + t * 40},${140 - t * 50},${80 + t * 20})`;
    ctx.beginPath();
    ctx.arc(s.x, s.y, BODY_R - t * 2, 0, Math.PI * 2);
    ctx.fill();
  }
  const head = body[0];
  const hc = cellCenter(head.x, head.y);
  const hs = worldToScreen(game, hc.x, hc.y);
  if (inView(game, hs.x, hs.y, 40)) {
    ctx.fillStyle = game.snake.mode === "recover" ? "#c07070" : game.snake.anger > 0 ? "#ff3a45" : "#e82230";
    ctx.beginPath();
    ctx.arc(hs.x, hs.y, HEAD_R, 0, Math.PI * 2);
    ctx.fill();
    const eye = 4;
    const dx = game.snake.dir.x;
    const dy = game.snake.dir.y;
    ctx.fillStyle = "#fff";
    ctx.beginPath();
    ctx.arc(hs.x + dx * 5 - dy * 4, hs.y + dy * 5 + dx * 4, eye, 0, Math.PI * 2);
    ctx.arc(hs.x + dx * 5 + dy * 4, hs.y + dy * 5 - dx * 4, eye, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = "#111";
    ctx.beginPath();
    ctx.arc(hs.x + dx * 6 - dy * 4, hs.y + dy * 6 + dx * 4, 1.8, 0, Math.PI * 2);
    ctx.arc(hs.x + dx * 6 + dy * 4, hs.y + dy * 6 - dx * 4, 1.8, 0, Math.PI * 2);
    ctx.fill();
  }
}

function drawPlayer(ctx: CanvasRenderingContext2D, game: Game): void {
  const p = game.player;
  for (let i = 0; i < p.trail.length; i++) {
    const t = p.trail[i];
    const s = worldToScreen(game, t.x, t.y);
    ctx.globalAlpha = (i + 1) / (p.trail.length + 2) * 0.35;
    ctx.fillStyle = "#7ad7ff";
    ctx.beginPath();
    ctx.arc(s.x, s.y, PLAYER_R * 0.7, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
  const s = worldToScreen(game, p.x, p.y);
  if (p.dash > 0) {
    ctx.strokeStyle = "rgba(122, 215, 255, 0.7)";
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.arc(s.x, s.y, PLAYER_R + 6, 0, Math.PI * 2);
    ctx.stroke();
  }
  if (p.boost > 0) {
    ctx.strokeStyle = "rgba(255, 209, 92, 0.65)";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(s.x, s.y, PLAYER_R + 9, 0, Math.PI * 2);
    ctx.stroke();
  }
  ctx.fillStyle = "#7ad7ff";
  ctx.beginPath();
  ctx.arc(s.x, s.y, PLAYER_R, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = "#e8fbff";
  ctx.beginPath();
  ctx.arc(s.x + p.facingX * 3, s.y + p.facingY * 3, 3.5, 0, Math.PI * 2);
  ctx.fill();
}

function drawFx(ctx: CanvasRenderingContext2D, game: Game): void {
  for (const p of game.particles) {
    const s = worldToScreen(game, p.x, p.y);
    ctx.globalAlpha = Math.max(0, p.life / p.max);
    ctx.fillStyle = p.color;
    ctx.beginPath();
    ctx.arc(s.x, s.y, p.r, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.font = "600 14px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  for (const f of game.floaters) {
    const s = worldToScreen(game, f.x, f.y);
    ctx.globalAlpha = Math.max(0, Math.min(1, f.life * 2));
    ctx.fillStyle = "#fff";
    ctx.fillText(f.text, s.x, s.y);
  }
  ctx.globalAlpha = 1;
}

function drawMinimap(ctx: CanvasRenderingContext2D, game: Game): void {
  const size = MINIMAP_SIZE;
  const x = MINIMAP_PAD;
  const y = MINIMAP_PAD;
  ctx.fillStyle = "rgba(10, 14, 20, 0.88)";
  round(ctx, x, y, size, size, 8);
  ctx.fill();
  ctx.strokeStyle = "#3e4d63";
  ctx.lineWidth = 1.5;
  ctx.stroke();

  const sx = size / WORLD_W;
  const sy = size / WORLD_H;
  const mapX = (wx: number) => x + wx * sx;
  const mapY = (wy: number) => y + wy * sy;

  ctx.strokeStyle = "rgba(255,255,255,0.25)";
  ctx.lineWidth = 1;
  ctx.strokeRect(mapX(game.camX), mapY(game.camY), game.view.viewportW * sx, game.view.viewportH * sy);

  ctx.fillStyle = "#ffd15c";
  for (const fruit of game.fruits) {
    const c = cellCenter(fruit.x, fruit.y);
    ctx.fillRect(mapX(c.x) - 1, mapY(c.y) - 1, 2, 2);
  }

  const body = game.snake.body;
  ctx.strokeStyle = "#2f9e5a";
  ctx.lineWidth = 2;
  ctx.beginPath();
  for (let i = 0; i < body.length; i++) {
    const c = cellCenter(body[i].x, body[i].y);
    if (i === 0) ctx.moveTo(mapX(c.x), mapY(c.y));
    else ctx.lineTo(mapX(c.x), mapY(c.y));
  }
  ctx.stroke();

  if (body[0]) {
    const c = cellCenter(body[0].x, body[0].y);
    const pulse =
      game.snake.mode === "windup" || game.snake.mode === "lunge"
        ? 3.2 + Math.sin(game.time * 20)
        : 2.4;
    ctx.fillStyle = game.snake.mode === "recover" ? "#c0a0a0" : "#ff2a35";
    ctx.beginPath();
    ctx.arc(mapX(c.x), mapY(c.y), pulse, 0, Math.PI * 2);
    ctx.fill();
  }

  ctx.fillStyle = "#7ad7ff";
  ctx.beginPath();
  ctx.arc(mapX(game.player.x), mapY(game.player.y), 2.6, 0, Math.PI * 2);
  ctx.fill();
}

function drawHud(ctx: CanvasRenderingContext2D, game: Game): void {
  const { screenW, screenH, wide } = game.view;
  if (wide) {
    ctx.fillStyle = "rgba(8, 12, 18, 0.55)";
    ctx.fillRect(0, 0, screenW, 58);
    ctx.fillRect(0, screenH - 36, screenW, 36);
  }

  const left = MINIMAP_PAD + MINIMAP_SIZE + 16;
  ctx.textAlign = "left";
  ctx.textBaseline = "middle";
  ctx.fillStyle = "#e8eef7";
  ctx.font = "600 15px sans-serif";
  ctx.fillText(`存活 ${formatTime(game.time)}`, left, 22);
  ctx.fillText(`得分 ${game.score}`, left + 150, 22);
  ctx.fillText(`蛇长 ${game.snake.body.length}`, left + 280, 22);

  const mode = game.snake.mode;
  let status = "搜寻中";
  let color = "#9aa8bd";
  if (mode === "windup") {
    status = "扑咬预警！";
    color = "#ffb04a";
  } else if (mode === "lunge") {
    status = "扑咬中";
    color = "#ff6b6b";
  } else if (mode === "recover") {
    status = "扑空硬直";
    color = "#8fd4ff";
  } else if (game.snake.anger > 0) {
    status = "狂暴追击";
    color = "#ff6b6b";
  } else if (game.time > 40) {
    status = "越追越快";
    color = "#ff8d8d";
  }
  ctx.fillStyle = color;
  ctx.font = "13px sans-serif";
  ctx.fillText(status, left + 400, 22);
  if (wide) {
    ctx.fillStyle = "#8ea0b8";
    ctx.fillText("全图视野 · Esc 退出", left + 520, 22);
  }

  ctx.fillStyle = "#8ea0b8";
  ctx.fillText(`最佳 ${formatTime(game.bestTime)}    最高分 ${game.bestScore}`, left, 48);

  ctx.textAlign = "center";
  ctx.fillStyle = "#7f91a8";
  ctx.font = "13px sans-serif";
  const foot = screenH - FOOT_H / 2;
  if (game.phase === "play") {
    const dash = game.player.dashCd > 0 ? "冲刺冷却中" : "空格冲刺";
    ctx.fillText(`WASD 移动    ${dash}    橙/红格=扑咬路线    侧闪躲开`, screenW / 2, foot);
  } else {
    ctx.fillText(
      wide
        ? "整张地图都在画面里，格子大小不变。Esc 回菜单。"
        : "蛇头碰到你就死。贴着蛇身走，看预警侧闪，把它引进自己的身体。",
      screenW / 2,
      foot,
    );
  }
  ctx.textAlign = "left";
}

function drawTitle(ctx: CanvasRenderingContext2D, game: Game): void {
  panel(ctx, game, 460, 320);
  const { screenW, originY, wide } = game.view;
  ctx.textAlign = "center";
  ctx.fillStyle = "#f4f7fb";
  ctx.font = "700 32px sans-serif";
  ctx.fillText(wide ? "全图视野" : "反向贪吃蛇", screenW / 2, originY + 120);
  ctx.font = "15px sans-serif";
  ctx.fillStyle = "#d5deea";
  const lines = wide
    ? [
        "世界地图仍是 56×36 格，格子大小不变。",
        "不再用小窗口跟镜头，整张地图同时显示。",
        "规则与猎人追逐相同：躲开扑咬，诱蛇断身。",
        "Esc 退出并回菜单。",
      ]
    : [
        "地图比屏幕大，镜头跟着你。左上角是小地图。",
        "蛇会靠近后锁定扑咬——看红格预警，侧闪躲开。",
        "扑空后它会硬直。蛇身是墙，可诱它撞自己或边界断身。",
        "金色果子：你吃加速，蛇吃变长。",
      ];
  lines.forEach((line, i) => ctx.fillText(line, screenW / 2, originY + 168 + i * 26));
  ctx.fillStyle = "#ffd15c";
  ctx.font = "600 16px sans-serif";
  ctx.fillText("Enter / 点击开始", screenW / 2, originY + 300);
}

function drawDead(ctx: CanvasRenderingContext2D, game: Game): void {
  panel(ctx, game, 380, 250);
  const { screenW, originY } = game.view;
  ctx.textAlign = "center";
  ctx.fillStyle = "#ffd0d0";
  ctx.font = "700 30px sans-serif";
  ctx.fillText("你被咬到了", screenW / 2, originY + 150);
  ctx.fillStyle = "#e8eef7";
  ctx.font = "16px sans-serif";
  ctx.fillText(`存活 ${formatTime(game.time)}`, screenW / 2, originY + 194);
  ctx.fillText(`得分 ${game.score}`, screenW / 2, originY + 222);
  ctx.fillStyle = "#ffd15c";
  ctx.font = "600 16px sans-serif";
  ctx.fillText("Enter / 点击 再来一局", screenW / 2, originY + 268);
}

function panel(ctx: CanvasRenderingContext2D, game: Game, w: number, h: number): void {
  const { screenW, originX, originY, viewportW, viewportH } = game.view;
  const x = (screenW - w) / 2;
  const y = originY + (viewportH - h) / 2;
  ctx.fillStyle = "rgba(8, 10, 14, 0.72)";
  ctx.fillRect(originX, originY, viewportW, viewportH);
  ctx.fillStyle = "rgba(18, 24, 34, 0.94)";
  round(ctx, x, y, w, h, 16);
  ctx.fill();
  ctx.strokeStyle = "#3e4d63";
  ctx.lineWidth = 2;
  ctx.stroke();
}

function round(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, r);
}

function inView(game: Game, x: number, y: number, pad: number): boolean {
  const { originX, originY, viewportW, viewportH } = game.view;
  return (
    x >= originX - pad &&
    x <= originX + viewportW + pad &&
    y >= originY - pad &&
    y <= originY + viewportH + pad
  );
}
