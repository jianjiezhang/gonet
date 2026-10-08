import {
  BODY_R,
  CELL,
  COLS,
  DASH_CD,
  FOOT_H,
  HEAD_R,
  MINIMAP_PAD,
  MINIMAP_SIZE,
  PLAYER_R,
  WORLD_H,
  WORLD_W,
  ROWS,
  cellCenter,
} from "./config";
import { ARENA_TARGET_COUNT, BEACON_RADIUS } from "./missionConfig";
import {
  finishRanking,
  formatTime,
  isProtectedAt,
  worldToScreen,
  type MissionGame,
  type SafeZone,
} from "./missionGame";

export function drawMission(ctx: CanvasRenderingContext2D, game: MissionGame): void {
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
  drawSafes(ctx, game);
  drawBeacons(ctx, game);
  for (const snake of game.snakes) drawSnake(ctx, game, snake);
  drawBots(ctx, game);
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
    ctx.fillStyle = `rgba(255,64,64,${game.flash * 0.4})`;
    ctx.fillRect(0, 0, screenW, screenH);
  }
  if (game.phase === "brief") drawBrief(ctx, game);
  if (game.phase === "win") drawEnd(ctx, game, "任务完成", "#b6ff9a");
  if (game.phase === "dead") drawEnd(ctx, game, "你被咬到了", "#ffd0d0");
  if (game.phase === "timeout") {
    drawEnd(ctx, game, game.view.wide ? "未及时到达目标" : "时间到", "#ffd15c");
  }
}

function drawArena(ctx: CanvasRenderingContext2D, game: MissionGame): void {
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
  ctx.strokeStyle = "#5a6f8c";
  ctx.lineWidth = 4;
  ctx.strokeRect(tl.x, tl.y, br.x - tl.x, br.y - tl.y);
}

function drawSafes(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  for (const zone of game.safes) {
    if (zone.phase === "cooldown") continue;
    const s = worldToScreen(game, zone.x, zone.y);
    const style = safeStyle(zone);
    ctx.beginPath();
    ctx.arc(s.x, s.y, zone.radius, 0, Math.PI * 2);
    ctx.fillStyle = style.fill;
    ctx.fill();
    ctx.strokeStyle = style.stroke;
    ctx.lineWidth = zone.phase === "unstable" ? 2 + Math.sin(game.elapsed * 18) : 2;
    ctx.stroke();
    ctx.fillStyle = style.text;
    ctx.font = "12px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(style.label, s.x, s.y + 4);
  }
}

function safeStyle(zone: SafeZone): { fill: string; stroke: string; text: string; label: string } {
  if (zone.start) {
    return {
      fill: "rgba(70, 140, 255, 0.22)",
      stroke: "#7eb6ff",
      text: "#e7f3ff",
      label: "开始",
    };
  }
  if (zone.mark === "safe") {
    return {
      fill: "rgba(80, 180, 160, 0.22)",
      stroke: "#5ec8b0",
      text: "#b8fff0",
      label: "安全区",
    };
  }
  switch (zone.phase) {
    case "ready":
      return {
        fill: "rgba(80, 180, 160, 0.18)",
        stroke: "#5ec8b0",
        text: "#b8fff0",
        label: "安全区",
      };
    case "shelter":
      return {
        fill: "rgba(70, 200, 170, 0.28)",
        stroke: "#4dffc8",
        text: "#e8fff8",
        label: Number.isFinite(zone.phaseT) ? `庇护 ${Math.ceil(zone.phaseT)}` : "庇护",
      };
    case "unstable":
      return {
        fill: "rgba(255, 170, 60, 0.22)",
        stroke: "#ffb04a",
        text: "#ffe0b0",
        label: `不稳 ${Math.ceil(zone.phaseT)}`,
      };
    case "collapse":
      return {
        fill: "rgba(120, 120, 130, 0.15)",
        stroke: "#889",
        text: "#ccc",
        label: "瓦解",
      };
    default:
      return { fill: "transparent", stroke: "#444", text: "#888", label: "" };
  }
}

function drawBeacons(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  if (game.view.wide) {
    const beacon = game.beacons[0];
    if (!beacon) return;
    const s = worldToScreen(game, beacon.x, beacon.y);
    ctx.beginPath();
    ctx.arc(s.x, s.y, BEACON_RADIUS, 0, Math.PI * 2);
    ctx.fillStyle = "rgba(255, 200, 60, 0.3)";
    ctx.fill();
    ctx.strokeStyle = "#ffd15c";
    ctx.lineWidth = 3;
    ctx.stroke();
    ctx.fillStyle = "#fff";
    ctx.font = "bold 16px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(beacon.name, s.x, s.y + 5);
    return;
  }
  game.beacons.forEach((beacon, i) => {
    const s = worldToScreen(game, beacon.x, beacon.y);
    const current = i === game.playerBeacon;
    const done = i < game.playerBeacon;
    ctx.beginPath();
    ctx.arc(s.x, s.y, BEACON_RADIUS, 0, Math.PI * 2);
    ctx.fillStyle = done
      ? "rgba(100, 200, 120, 0.25)"
      : current
        ? "rgba(255, 200, 60, 0.3)"
        : "rgba(120, 140, 160, 0.15)";
    ctx.fill();
    ctx.strokeStyle = done ? "#6dcf7a" : current ? "#ffd15c" : "#6a7d99";
    ctx.lineWidth = current ? 3 : 1.5;
    ctx.stroke();
    ctx.fillStyle = "#fff";
    ctx.font = "bold 16px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(beacon.name, s.x, s.y + 5);
    if (current && game.playerStand > 0) {
      ctx.strokeStyle = "#fff";
      ctx.beginPath();
      ctx.arc(
        s.x,
        s.y,
        BEACON_RADIUS + 6,
        -Math.PI / 2,
        -Math.PI / 2 + Math.PI * 2 * game.playerStand,
      );
      ctx.stroke();
    }
  });
}

function drawSnake(
  ctx: CanvasRenderingContext2D,
  game: MissionGame,
  snake: MissionGame["snakes"][number],
): void {
  const { body, dir, mode } = snake;
  if (!body.length) return;
  ctx.lineCap = "round";
  ctx.strokeStyle = "#0d3d28";
  ctx.lineWidth = BODY_R * 2.1;
  ctx.beginPath();
  body.forEach((seg, i) => {
    const c = cellCenter(seg.x, seg.y);
    const s = worldToScreen(game, c.x, c.y);
    if (i === 0) ctx.moveTo(s.x, s.y);
    else ctx.lineTo(s.x, s.y);
  });
  ctx.stroke();
  for (let i = body.length - 1; i >= 1; i--) {
    const c = cellCenter(body[i].x, body[i].y);
    const s = worldToScreen(game, c.x, c.y);
    const t = i / body.length;
    ctx.fillStyle = t > 0.5 ? "#145a38" : "#1f8f4e";
    ctx.beginPath();
    ctx.arc(s.x, s.y, BODY_R * (1 - t * 0.3), 0, Math.PI * 2);
    ctx.fill();
  }

  const hc = cellCenter(body[0].x, body[0].y);
  const hs = worldToScreen(game, hc.x, hc.y);
  const tip = HEAD_R + (mode === "lunge" || mode === "windup" ? 5 : 2);
  const px = -dir.y;
  const py = dir.x;
  ctx.beginPath();
  ctx.moveTo(hs.x + dir.x * tip, hs.y + dir.y * tip);
  ctx.lineTo(
    hs.x - dir.x * HEAD_R * 0.85 + px * HEAD_R,
    hs.y - dir.y * HEAD_R * 0.85 + py * HEAD_R,
  );
  ctx.lineTo(
    hs.x - dir.x * HEAD_R * 0.85 - px * HEAD_R,
    hs.y - dir.y * HEAD_R * 0.85 - py * HEAD_R,
  );
  ctx.closePath();
  ctx.fillStyle = mode === "recover" ? "#c9a0a0" : "#e01222";
  ctx.fill();
}

function drawBots(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  for (const bot of game.bots) {
    if (!bot.alive) continue;
    const s = worldToScreen(game, bot.x, bot.y);
    ctx.fillStyle = bot.color;
    ctx.beginPath();
    ctx.arc(s.x, s.y, PLAYER_R - 1, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = "rgba(255,255,255,0.5)";
    ctx.stroke();
  }
}

function drawPlayer(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  const p = game.player;
  if (!p.alive) return;
  const s = worldToScreen(game, p.x, p.y);
  if (isProtectedAt(game, p.x, p.y)) {
    ctx.strokeStyle = "rgba(120,255,220,0.7)";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(s.x, s.y, PLAYER_R + 7, 0, Math.PI * 2);
    ctx.stroke();
  }
  ctx.fillStyle = "#7ad7ff";
  ctx.beginPath();
  ctx.arc(s.x, s.y, PLAYER_R, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = "#e8fbff";
  ctx.lineWidth = 2;
  ctx.stroke();
  ctx.fillStyle = "#3dff6a";
  ctx.font = "700 16px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "bottom";
  ctx.fillText("我", s.x, s.y - PLAYER_R - 4);
  ctx.textBaseline = "alphabetic";
  if (p.dashCd > 0) {
    ctx.strokeStyle = "rgba(255,255,255,0.7)";
    ctx.beginPath();
    ctx.arc(
      s.x,
      s.y,
      PLAYER_R + 5,
      -Math.PI / 2,
      -Math.PI / 2 + Math.PI * 2 * (1 - p.dashCd / DASH_CD),
    );
    ctx.stroke();
  }
}

function drawFx(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  for (const p of game.particles) {
    const s = worldToScreen(game, p.x, p.y);
    ctx.globalAlpha = Math.max(0, p.life / p.max);
    ctx.fillStyle = p.color;
    ctx.beginPath();
    ctx.arc(s.x, s.y, p.r, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
  ctx.font = "bold 14px sans-serif";
  ctx.textAlign = "center";
  for (const f of game.floaters) {
    const s = worldToScreen(game, f.x, f.y);
    ctx.globalAlpha = Math.min(1, f.life * 2);
    ctx.fillStyle = "#fff";
    ctx.fillText(f.text, s.x, s.y);
  }
  ctx.globalAlpha = 1;
}

function drawMinimap(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  const size = MINIMAP_SIZE;
  const x = MINIMAP_PAD;
  const y = MINIMAP_PAD + 18;
  // 标题条
  ctx.fillStyle = "rgba(20, 40, 55, 0.95)";
  round(ctx, x, MINIMAP_PAD, size, 16, 6);
  ctx.fill();
  ctx.fillStyle = "#7ad7ff";
  ctx.font = "600 11px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("小地图 · 全场概览", x + size / 2, MINIMAP_PAD + 8);

  ctx.fillStyle = "rgba(8, 12, 18, 0.94)";
  round(ctx, x, y, size, size, 8);
  ctx.fill();
  ctx.strokeStyle = "#7ad7ff";
  ctx.lineWidth = 2.5;
  ctx.stroke();
  // 内边框
  ctx.strokeStyle = "rgba(255,255,255,0.12)";
  ctx.lineWidth = 1;
  ctx.strokeRect(x + 3, y + 3, size - 6, size - 6);

  const sx = size / WORLD_W;
  const sy = size / WORLD_H;
  const mx = (wx: number) => x + wx * sx;
  const my = (wy: number) => y + wy * sy;

  const viewW = game.view.viewportW;
  const viewH = game.view.viewportH;
  if (viewW < WORLD_W || viewH < WORLD_H) {
    ctx.fillStyle = "rgba(122, 215, 255, 0.08)";
    ctx.fillRect(mx(game.camX), my(game.camY), viewW * sx, viewH * sy);
    ctx.strokeStyle = "rgba(122, 215, 255, 0.85)";
    ctx.lineWidth = 1.5;
    ctx.strokeRect(mx(game.camX), my(game.camY), viewW * sx, viewH * sy);
  }

  for (const zone of game.safes) {
    if (zone.phase === "cooldown") continue;
    ctx.fillStyle = zone.start
      ? "#7eb6ff"
      : zone.mark === "safe"
        ? "#5ec8b0"
      : zone.phase === "unstable"
        ? "#ffb04a"
        : zone.phase === "collapse"
          ? "#777"
          : "#5ec8b0";
    ctx.beginPath();
    ctx.arc(mx(zone.x), my(zone.y), Math.max(5, zone.radius * sx), 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = "rgba(255,255,255,0.35)";
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  const marks = game.view.wide ? game.beacons.slice(0, 1) : game.beacons;
  marks.forEach((b, i) => {
    const current = game.view.wide || i === game.playerBeacon;
    ctx.fillStyle = current ? "#ffd15c" : !game.view.wide && i < game.playerBeacon ? "#6dcf7a" : "#9aa8bd";
    const r = current ? 4 : 3;
    ctx.beginPath();
    ctx.arc(mx(b.x), my(b.y), r, 0, Math.PI * 2);
    ctx.fill();
    if (current) {
      ctx.strokeStyle = "#fff";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.arc(mx(b.x), my(b.y), 6, 0, Math.PI * 2);
      ctx.stroke();
    }
  });

  for (const snake of game.snakes) {
    const body = snake.body;
    ctx.strokeStyle = "#3dce73";
    ctx.lineWidth = 2.5;
    ctx.beginPath();
    body.forEach((seg, i) => {
      const c = cellCenter(seg.x, seg.y);
      if (i === 0) ctx.moveTo(mx(c.x), my(c.y));
      else ctx.lineTo(mx(c.x), my(c.y));
    });
    ctx.stroke();
    if (body[0]) {
      const c = cellCenter(body[0].x, body[0].y);
      ctx.fillStyle = "#ff2a35";
      ctx.beginPath();
      ctx.arc(mx(c.x), my(c.y), 3.5, 0, Math.PI * 2);
      ctx.fill();
      ctx.strokeStyle = "#ffd0d0";
      ctx.lineWidth = 1;
      ctx.stroke();
    }
  }

  for (const bot of game.bots) {
    if (!bot.alive) continue;
    ctx.fillStyle = bot.color;
    ctx.beginPath();
    ctx.arc(mx(bot.x), my(bot.y), 2.4, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.fillStyle = "#7ad7ff";
  ctx.beginPath();
  ctx.arc(mx(game.player.x), my(game.player.y), 3.4, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = "#fff";
  ctx.lineWidth = 1.2;
  ctx.stroke();

  // 图例
  const ly = y + size + 4;
  ctx.fillStyle = "rgba(10,14,20,0.9)";
  round(ctx, x, ly, size, 36, 6);
  ctx.fill();
  ctx.font = "10px sans-serif";
  ctx.textAlign = "left";
  ctx.fillStyle = "#7ad7ff";
  ctx.fillText("●你", x + 6, ly + 12);
  ctx.fillStyle = "#ff2a35";
  ctx.fillText("●蛇头", x + 36, ly + 12);
  ctx.fillStyle = "#5ec8b0";
  ctx.fillText("●安全区", x + 78, ly + 12);
  ctx.fillStyle = "#ffd15c";
  ctx.fillText("●当前信标", x + 6, ly + 26);
  ctx.fillStyle = "#9aa8bd";
  ctx.fillText(game.view.wide ? "全图" : "□视野", x + 78, ly + 26);
}

function drawHud(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  const left = MINIMAP_PAD + MINIMAP_SIZE + 16;
  ctx.textAlign = "left";
  ctx.textBaseline = "middle";
  ctx.fillStyle = "#e8eef7";
  ctx.font = "600 14px sans-serif";
  ctx.fillText(
    game.view.wide ? `本站 ${formatTime(game.timeLeft)}` : `剩余 ${formatTime(game.timeLeft)}`,
    left,
    22,
  );
  ctx.fillText(`得分 ${game.score}`, left + 120, 22);
  ctx.fillStyle = "#9aa8bd";
  ctx.font = "12px sans-serif";
  ctx.fillText(`最高 ${game.bestScore} · Esc 菜单`, left, 46);

  drawTaskPanel(ctx, game);

  ctx.textAlign = "center";
  ctx.fillStyle = "#7f91a8";
  ctx.font = "13px sans-serif";
  ctx.fillText(
    game.phase === "play"
      ? game.view.wide
        ? "WASD 移动  空格冲刺  底部开始区出发  一次一个目标  碰满 5 个"
        : "WASD 移动  空格冲刺  青区=安全(会变)  黄圈=信标  终点不能在区内交"
      : "Esc 返回模式选择",
    game.view.screenW / 2,
    game.view.screenH - FOOT_H / 2,
  );
}

function drawTaskPanel(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  const w = 210;
  const h = 128;
  const x = game.view.screenW - game.view.originX - w - (game.view.wide ? 12 : 0);
  const y = 6;
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, 10);
  ctx.fillStyle = "rgba(14, 20, 30, 0.92)";
  ctx.fill();
  ctx.strokeStyle = "#3e4d63";
  ctx.lineWidth = 1.5;
  ctx.stroke();

  ctx.textAlign = "left";
  ctx.textBaseline = "middle";
  ctx.fillStyle = "#ffd15c";
  ctx.font = "600 13px sans-serif";
  ctx.fillText("任务", x + 12, y + 16);

  ctx.fillStyle = "#c5d0de";
  ctx.font = "12px sans-serif";
  ctx.fillText(game.view.wide ? "总目标：碰到 5 个目标" : "总目标：信标 A → B → C", x + 12, y + 36);

  const done = game.playerBeacon;
  const total = game.view.wide ? ARENA_TARGET_COUNT : game.beacons.length;
  let current = game.view.wide ? "已完成全部目标" : "已完成全部信标";
  if (done < total) {
    const b = game.view.wide ? game.beacons[0] : game.beacons[done];
    current = !b
      ? current
      : game.view.wide
        ? `当前：目标 ${b.name}（${Math.ceil(game.objectiveTimeLeft)}s）`
        : game.playerStand > 0
          ? `当前：站稳信标 ${b.name}（${Math.min(100, Math.floor(game.playerStand * 100))}%）`
          : `当前：前往信标 ${b.name}`;
  }
  ctx.fillStyle = "#e8eef7";
  ctx.font = "600 12px sans-serif";
  ctx.fillText(current, x + 12, y + 56);

  ctx.fillStyle = "#9aa8bd";
  ctx.font = "12px sans-serif";
  ctx.fillText(
    game.view.wide ? `全场进度：${Math.min(done, total)} / ${total}` : `自己进度：${Math.min(done, total)} / ${total}`,
    x + 12,
    y + 76,
  );

  const shielded = game.player.alive && isProtectedAt(game, game.player.x, game.player.y);
  const status = !game.player.alive
    ? "状态：已被咬"
    : shielded
      ? "状态：安全区内"
      : "状态：暴露（蛇可能追你）";
  ctx.fillStyle = shielded ? "#7dffc8" : "#ffb0b0";
  ctx.fillText(status, x + 12, y + 96);

  // 机器人简况
  const botDone = game.bots.filter((b) => b.finishTime >= 0).length;
  const botAlive = game.bots.filter((b) => b.alive).length;
  ctx.fillStyle = "#8ea0b8";
  ctx.font = "11px sans-serif";
  ctx.fillText(`机器人 存活${botAlive} 送达${botDone}`, x + 12, y + 112);
}

function drawBrief(ctx: CanvasRenderingContext2D, game: MissionGame): void {
  const h = 360;
  panel(ctx, game, 520, h);
  const y = panelTop(game, h);
  const cx = game.view.screenW / 2;
  ctx.textAlign = "center";
  ctx.fillStyle = "#f4f7fb";
  ctx.font = "700 28px sans-serif";
  ctx.fillText(game.view.wide ? "猎场信使 · 全图视野" : "本局玩法 · 猎场信使", cx, y + 48);
  ctx.font = "14px sans-serif";
  ctx.fillStyle = "#d5deea";
  const lines = game.view.wide
    ? [
        "底部蓝色开始区集合出发。另有一处安全区和一处庇护区，都一直安全。",
        "场上一次只出现一个目标，位置随机。碰到后才出现下一个。",
        "每个目标限时 30 秒，碰满 5 个才算完成，超时失败。",
        "离猎物近：60% 锁最近的区外猎物，40% 锁更远的。",
        "离得远：40% 最近，40% 更远，20% 闲逛。锁定后一直追到对方进区或倒下。",
      ]
    : [
        "限时 3 分钟，按顺序站上信标 A → B → C（每站约 1 秒）。",
        "左上角小地图看全场：青=你，红=蛇头，绿圈=安全区，黄=当前信标。",
        "安全区可躲蛇，但会庇护→不稳→瓦解→换位，不能久蹲。",
        "最后一站不能在安全区内完成。蛇会追最近的区外猎物。",
        "你或任意机器人跑完 A→B→C，都算成功结束。",
      ];
  lines.forEach((line, i) => ctx.fillText(line, cx, y + 96 + i * 28));
  ctx.fillStyle = "#ffd15c";
  ctx.font = "600 17px sans-serif";
  ctx.fillText("看完后按 Enter / 点击 / 空格 开始", cx, y + h - 52);
}

function drawEnd(
  ctx: CanvasRenderingContext2D,
  game: MissionGame,
  title: string,
  color: string,
): void {
  const h = 300;
  panel(ctx, game, 420, h);
  const y = panelTop(game, h);
  const cx = game.view.screenW / 2;
  ctx.textAlign = "center";
  ctx.fillStyle = color;
  ctx.font = "700 28px sans-serif";
  ctx.fillText(title, cx, y + 48);
  ctx.fillStyle = "#e8eef7";
  ctx.font = "16px sans-serif";
  ctx.fillText(`得分 ${game.score}`, cx, y + 88);
  const ranks = finishRanking(game);
  if (ranks.length) {
    ctx.font = "14px sans-serif";
    ctx.fillStyle = "#b8c4d4";
    ranks.slice(0, 5).forEach((r, i) => {
      ctx.fillText(`${i + 1}. ${r.name}  ${r.time.toFixed(1)}s`, cx, y + 118 + i * 20);
    });
  }
  ctx.fillStyle = "#ffd15c";
  ctx.font = "600 15px sans-serif";
  ctx.fillText("Enter 再来    Esc 回菜单", cx, y + h - 36);
}

function panelTop(game: MissionGame, h: number): number {
  return game.view.originY + (game.view.viewportH - h) / 2;
}

function panel(ctx: CanvasRenderingContext2D, game: MissionGame, w: number, h: number): void {
  const { screenW, originX, originY, viewportW, viewportH } = game.view;
  const x = (screenW - w) / 2;
  const y = panelTop(game, h);
  ctx.fillStyle = "rgba(8,10,14,0.72)";
  ctx.fillRect(originX, originY, viewportW, viewportH);
  ctx.fillStyle = "rgba(18,24,34,0.94)";
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
