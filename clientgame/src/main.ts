import "./style.css";
import {
  agreeFriend,
  applyFriend,
  deleteFriend,
  friendLine,
  friendNoticeText,
  loadFriends,
  rejectFriend,
  type FriendList,
  type FriendPerson,
  type FriendRequestRow,
} from "./friends";
import {
  agreeGuild,
  applyGuild,
  catalogLine,
  createGuild,
  disbandGuild,
  findGuildByID,
  findGuildByName,
  guildNoticeText,
  kickGuild,
  leaveGuild,
  loadGuildCatalog,
  loadMyGuild,
  memberLine,
  rejectGuild,
  type GuildApply,
  type GuildBrief,
  type GuildMember,
  type MyGuild,
} from "./guilds";
import { claimMission, loadMissions, rewardLabel, statusLabel, type MissionRow } from "./missions";
import { genderLabel, loadRole, saveLevel } from "./profile";
import {
  Cmd,
  errText,
  openSession,
  parseBody,
  parseFriendNotice,
  parseGuildNotice,
  type Session,
} from "./session";
import { VIEW_H, VIEW_W, WORLD_H, WORLD_W } from "./config";
import { draw } from "./draw";
import { createGame, update, wideView, windowView, type Game, type Input as ChaseInput } from "./game";
import { createMenu, drawMenu, pickMenuAt, updateMenu, type AppMode, type MenuState } from "./menu";
import { drawMission } from "./missionDraw";
import { createMission, updateMission, type Input as MissionInput, type MissionGame } from "./missionGame";

const canvas = document.querySelector<HTMLCanvasElement>("#game");
if (!canvas) throw new Error("找不到画布");
const ctx = canvas.getContext("2d");
if (!ctx) throw new Error("找不到 2D 上下文");

type View = "login" | "home" | "profile" | "missions" | "friends" | "guild" | "play";
let view: View = "login";
let appMode: AppMode = "menu";
let menu: MenuState = createMenu();
let roleID = "";
let session: Session | null = null;
let friendBusy = false;
let guildBusy = false;
let joinedGuild = false;
let guildHit: GuildBrief | null = null;
let looping = false;
let chase: Game | null = null;
let mission: MissionGame | null = null;

const keys = new Set<string>();
let spaceEdge = false;
let enterEdge = false;
let restartEdge = false;
let clickEdge = false;
let escapeEdge = false;
let axisPulseY = 0;
let axisPulseX = 0;

if (import.meta.env.DEV) {
  (window as unknown as { __app: { mode: () => AppMode } }).__app = {
    mode: () => appMode,
  };
}

function resize(): void {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  if (appMode === "arena" && mission) {
    canvas!.width = Math.floor(WORLD_W * dpr);
    canvas!.height = Math.floor(WORLD_H * dpr);
    ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
    return;
  }
  canvas!.width = Math.floor(VIEW_W * dpr);
  canvas!.height = Math.floor(VIEW_H * dpr);
  ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
}

resize();
window.addEventListener("resize", resize);
document.addEventListener("fullscreenchange", () => {
  if (appMode === "arena") resize();
});

window.addEventListener("keydown", (event) => {
  if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
  const block = ["Space", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(event.code);
  if (block) event.preventDefault();
  if (event.repeat) {
    keys.add(event.code);
    return;
  }
  keys.add(event.code);
  if (event.code === "Space") spaceEdge = true;
  if (event.code === "Enter") enterEdge = true;
  if (event.code === "KeyR") restartEdge = true;
  if (event.code === "Escape") escapeEdge = true;
  if (event.code === "ArrowUp" || event.code === "KeyW") axisPulseY = -1;
  if (event.code === "ArrowDown" || event.code === "KeyS") axisPulseY = 1;
  if (event.code === "ArrowLeft" || event.code === "KeyA") axisPulseX = -1;
  if (event.code === "ArrowRight" || event.code === "KeyD") axisPulseX = 1;
});

window.addEventListener("keyup", (event) => {
  keys.delete(event.code);
});

window.addEventListener("blur", () => {
  keys.clear();
});

canvas.addEventListener("pointerdown", (event) => {
  clickEdge = true;
  if (appMode === "menu") {
    const rect = canvas!.getBoundingClientRect();
    const y = ((event.clientY - rect.top) / rect.height) * VIEW_H;
    menu.selected = pickMenuAt(y);
  }
});

let last = performance.now();

function frame(now: number): void {
  const dt = (now - last) / 1000;
  last = now;
  const axis = readAxis();
  if (axisPulseY !== 0) {
    axis.y = axisPulseY;
    axisPulseY = 0;
  }
  if (axisPulseX !== 0) {
    axis.x = axisPulseX;
    axisPulseX = 0;
  }

  const edges = {
    spaceEdge,
    enterEdge,
    restartEdge,
    clickEdge,
    escapeEdge,
  };
  spaceEdge = false;
  enterEdge = false;
  restartEdge = false;
  clickEdge = false;
  escapeEdge = false;

  if (view !== "play") {
    axisPulseX = 0;
    axisPulseY = 0;
    requestAnimationFrame(frame);
    return;
  }

  if (appMode === "menu") {
    if (edges.escapeEdge) {
      showHome();
      requestAnimationFrame(frame);
      return;
    }
    const pick = updateMenu(menu, {
      ax: axis.x,
      ay: axis.y,
      enterEdge: edges.enterEdge,
      clickEdge: edges.clickEdge,
      spaceEdge: edges.spaceEdge,
    });
    if (pick === "chase") {
      exitWidePlay();
      chase = createGame(windowView());
      mission = null;
      appMode = "chase";
      resize();
    } else if (pick === "mission") {
      exitWidePlay();
      mission = createMission();
      chase = null;
      appMode = "mission";
      resize();
    } else if (pick === "arena") {
      chase = null;
      appMode = "arena";
      mission = createMission(wideView());
      void enterWidePlay();
      resize();
    }
    if (appMode === "menu") drawMenu(ctx!, menu, roleID);
  } else if (appMode === "chase" && chase) {
    if (edges.escapeEdge) {
      exitWidePlay();
      appMode = "menu";
      chase = null;
      menu = createMenu();
      resize();
      drawMenu(ctx!, menu, roleID);
    } else {
      const input: ChaseInput = {
        ax: axis.x,
        ay: axis.y,
        spaceEdge: edges.spaceEdge,
        enterEdge: edges.enterEdge,
        restartEdge: edges.restartEdge,
        clickEdge: edges.clickEdge,
      };
      update(chase, input, dt);
      draw(ctx!, chase);
    }
  } else if ((appMode === "mission" || appMode === "arena") && mission) {
    const input: MissionInput = {
      ax: axis.x,
      ay: axis.y,
      spaceEdge: edges.spaceEdge,
      enterEdge: edges.enterEdge,
      restartEdge: edges.restartEdge,
      clickEdge: edges.clickEdge,
      escapeEdge: edges.escapeEdge,
    };
    const back = updateMission(mission, input, dt);
    if (back || edges.escapeEdge) {
      exitWidePlay();
      appMode = "menu";
      mission = null;
      menu = createMenu();
      resize();
      drawMenu(ctx!, menu, roleID);
    } else {
      drawMission(ctx!, mission);
    }
  }

  requestAnimationFrame(frame);
}

function readAxis(): { x: number; y: number } {
  const left = keys.has("KeyA") || keys.has("ArrowLeft");
  const right = keys.has("KeyD") || keys.has("ArrowRight");
  const up = keys.has("KeyW") || keys.has("ArrowUp");
  const down = keys.has("KeyS") || keys.has("ArrowDown");
  return {
    x: (right ? 1 : 0) - (left ? 1 : 0),
    y: (down ? 1 : 0) - (up ? 1 : 0),
  };
}

const home = document.querySelector<HTMLElement>("#home");
const homeRole = document.querySelector<HTMLElement>("#home-role");
const homeProfileBtn = document.querySelector<HTMLButtonElement>("#home-profile");
const homeMissionsBtn = document.querySelector<HTMLButtonElement>("#home-missions");
const homeFriendsBtn = document.querySelector<HTMLButtonElement>("#home-friends");
const homeGuildBtn = document.querySelector<HTMLButtonElement>("#home-guild");
const homePlayBtn = document.querySelector<HTMLButtonElement>("#home-play");
const missions = document.querySelector<HTMLElement>("#missions");
const missionsBack = document.querySelector<HTMLButtonElement>("#missions-back");
const missionList = document.querySelector<HTMLUListElement>("#mission-list");
const missionsNote = document.querySelector<HTMLParagraphElement>("#missions-note");
const missionsErr = document.querySelector<HTMLParagraphElement>("#missions-err");
const friends = document.querySelector<HTMLElement>("#friends");
const friendsBack = document.querySelector<HTMLButtonElement>("#friends-back");
const friendIdInput = document.querySelector<HTMLInputElement>("#friend-id");
const friendApplyBtn = document.querySelector<HTMLButtonElement>("#friend-apply");
const friendList = document.querySelector<HTMLUListElement>("#friend-list");
const friendIncoming = document.querySelector<HTMLUListElement>("#friend-incoming");
const friendOutgoing = document.querySelector<HTMLUListElement>("#friend-outgoing");
const friendsNote = document.querySelector<HTMLParagraphElement>("#friends-note");
const friendsErr = document.querySelector<HTMLParagraphElement>("#friends-err");
const guild = document.querySelector<HTMLElement>("#guild");
const guildBack = document.querySelector<HTMLButtonElement>("#guild-back");
const guildCreate = document.querySelector<HTMLElement>("#guild-create");
const guildNameInput = document.querySelector<HTMLInputElement>("#guild-name");
const guildNoticeInput = document.querySelector<HTMLInputElement>("#guild-notice");
const guildCreateBtn = document.querySelector<HTMLButtonElement>("#guild-create-btn");
const guildCard = document.querySelector<HTMLElement>("#guild-card");
const guildQuery = document.querySelector<HTMLInputElement>("#guild-query");
const guildFindIDBtn = document.querySelector<HTMLButtonElement>("#guild-find-id");
const guildFindNameBtn = document.querySelector<HTMLButtonElement>("#guild-find-name");
const guildResult = document.querySelector<HTMLUListElement>("#guild-result");
const guildCatalog = document.querySelector<HTMLUListElement>("#guild-catalog");
const guildNote = document.querySelector<HTMLParagraphElement>("#guild-note");
const guildErr = document.querySelector<HTMLParagraphElement>("#guild-err");
const profile = document.querySelector<HTMLElement>("#profile");
const profileBack = document.querySelector<HTMLButtonElement>("#profile-back");
const play = document.querySelector<HTMLElement>("#play");
const playBack = document.querySelector<HTMLButtonElement>("#play-back");
const profileId = document.querySelector<HTMLElement>("#profile-id");
const profileName = document.querySelector<HTMLElement>("#profile-name");
const profileLevel = document.querySelector<HTMLElement>("#profile-level");
const profileGender = document.querySelector<HTMLElement>("#profile-gender");
const profileErr = document.querySelector<HTMLParagraphElement>("#profile-err");
const levelInput = document.querySelector<HTMLInputElement>("#level");
const levelBtn = document.querySelector<HTMLButtonElement>("#level-btn");
const loginForm = document.querySelector<HTMLFormElement>("#login");
const roleInput = document.querySelector<HTMLInputElement>("#roleid");
const tokenInput = document.querySelector<HTMLInputElement>("#token");
const loginErr = document.querySelector<HTMLParagraphElement>("#login-err");
const loginBtn = document.querySelector<HTMLButtonElement>("#login-btn");

loginForm?.addEventListener("submit", (event) => {
  event.preventDefault();
  void submitLogin();
});

async function submitLogin(): Promise<void> {
  const id = roleInput?.value.trim() ?? "";
  const token = tokenInput?.value ?? "";
  if (!id) {
    if (loginErr) loginErr.textContent = "请填写角色号";
    return;
  }
  if (loginBtn) loginBtn.disabled = true;
  if (loginErr) loginErr.textContent = "";
  try {
    const next = await openSession();
    const raw = await next.request(Cmd.login, { roleid: id, token });
    const body = parseBody(raw);
    if (body !== "ok") {
      next.close();
      if (loginErr) loginErr.textContent = errText(body) || "登录失败";
      return;
    }
    session = next;
    session.onDrop = () => leave("连接已断开");
    session.on(Cmd.kick, () => leave("账号在别处登录"));
    session.on(Cmd.friendnotify, (data) => {
      const notice = parseFriendNotice(data);
      if (notice) onFriendNotice(notice.kind);
    });
    session.on(Cmd.guildnotify, (data) => {
      const notice = parseGuildNotice(data);
      if (notice) onGuildPush(notice.kind);
    });
    roleID = id;
    showHome();
  } catch (err) {
    session?.close();
    session = null;
    if (loginErr) loginErr.textContent = err instanceof Error ? err.message : "连不上游戏服";
  } finally {
    if (loginBtn) loginBtn.disabled = false;
  }
}

function startLoop(): void {
  if (looping) return;
  looping = true;
  last = performance.now();
  requestAnimationFrame(frame);
}

function leave(message: string): void {
  const current = session;
  session = null;
  roleID = "";
  exitWidePlay();
  appMode = "menu";
  chase = null;
  mission = null;
  menu = createMenu();
  current?.close();
  view = "login";
  if (home) home.hidden = true;
  if (profile) profile.hidden = true;
  if (missions) missions.hidden = true;
  if (friends) friends.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = true;
  if (loginForm) loginForm.hidden = false;
  if (loginErr) loginErr.textContent = message;
}

function clearInput(): void {
  keys.clear();
  spaceEdge = false;
  enterEdge = false;
  restartEdge = false;
  clickEdge = false;
  escapeEdge = false;
  axisPulseX = 0;
  axisPulseY = 0;
}

function showHome(): void {
  view = "home";
  appMode = "menu";
  chase = null;
  mission = null;
  menu = createMenu();
  clearInput();
  if (loginForm) loginForm.hidden = true;
  if (profile) profile.hidden = true;
  if (missions) missions.hidden = true;
  if (friends) friends.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = true;
  if (homeRole) homeRole.textContent = roleID ? `已登录 ${roleID}` : "";
  if (home) home.hidden = false;
}

function showProfile(): void {
  view = "profile";
  if (home) home.hidden = true;
  if (missions) missions.hidden = true;
  if (friends) friends.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = true;
  if (profile) profile.hidden = false;
  void refreshProfile();
}

function showPlay(): void {
  view = "play";
  exitWidePlay();
  appMode = "menu";
  chase = null;
  mission = null;
  menu = createMenu();
  clearInput();
  if (home) home.hidden = true;
  if (profile) profile.hidden = true;
  if (missions) missions.hidden = true;
  if (friends) friends.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = false;
  resize();
  drawMenu(ctx!, menu, roleID);
  startLoop();
}

function backFromPlay(): void {
  if (appMode !== "menu") {
    exitWidePlay();
    appMode = "menu";
    chase = null;
    mission = null;
    menu = createMenu();
    resize();
    drawMenu(ctx!, menu, roleID);
    return;
  }
  showHome();
}

async function enterWidePlay(): Promise<void> {
  play?.classList.add("wide");
  try {
    if (play && !document.fullscreenElement) await play.requestFullscreen();
  } catch {
    /* 浏览器拒绝全屏时仍用铺满窗口的布局 */
  }
}

function exitWidePlay(): void {
  play?.classList.remove("wide");
  if (document.fullscreenElement) {
    void document.exitFullscreen().catch(() => undefined);
  }
}

homeProfileBtn?.addEventListener("click", () => {
  showProfile();
});
homeMissionsBtn?.addEventListener("click", () => {
  showMissions();
});
homeFriendsBtn?.addEventListener("click", () => {
  showFriends();
});
homeGuildBtn?.addEventListener("click", () => {
  showGuild();
});
homePlayBtn?.addEventListener("click", () => {
  showPlay();
});
profileBack?.addEventListener("click", () => {
  showHome();
});
missionsBack?.addEventListener("click", () => {
  showHome();
});
friendsBack?.addEventListener("click", () => {
  showHome();
});
guildBack?.addEventListener("click", () => {
  showHome();
});
playBack?.addEventListener("click", () => {
  backFromPlay();
});
friendApplyBtn?.addEventListener("click", () => {
  void sendFriendApply();
});
guildCreateBtn?.addEventListener("click", () => {
  void sendGuildCreate();
});
guildFindIDBtn?.addEventListener("click", () => {
  void sendGuildFind("id");
});
guildFindNameBtn?.addEventListener("click", () => {
  void sendGuildFind("name");
});

async function refreshProfile(): Promise<void> {
  if (!session) return;
  try {
    const card = await loadRole(session);
    if (profileId) profileId.textContent = card.roleid;
    if (profileName) profileName.textContent = card.name;
    if (profileLevel) profileLevel.textContent = String(card.level);
    if (profileGender) profileGender.textContent = genderLabel(card.gender);
    if (levelInput) levelInput.value = String(card.level);
    if (profileErr) profileErr.textContent = "";
  } catch (err) {
    if (profileErr) profileErr.textContent = err instanceof Error ? err.message : "资料读取失败";
  }
}

function showMissions(): void {
  view = "missions";
  if (home) home.hidden = true;
  if (profile) profile.hidden = true;
  if (friends) friends.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = true;
  if (missionsNote) missionsNote.textContent = "";
  if (missionsErr) missionsErr.textContent = "";
  if (missions) missions.hidden = false;
  void refreshMissions();
}

function renderMissions(list: MissionRow[]): void {
  if (!missionList) return;
  missionList.replaceChildren();
  if (list.length === 0) {
    const item = document.createElement("li");
    item.textContent = "暂无任务";
    missionList.append(item);
    return;
  }
  for (const row of list) {
    const item = document.createElement("li");
    const text = document.createElement("span");
    text.className = "mission-text";
    const title = document.createElement("span");
    title.textContent = `任务 ${row.id}　${row.progress}/${row.target}　${statusLabel(row.status)}`;
    const reward = document.createElement("span");
    reward.className = "mission-reward";
    reward.textContent = rewardLabel(row.reward);
    text.append(title, reward);
    item.append(text);
    if (row.status === 2) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = "领取";
      button.addEventListener("click", () => {
        void claim(row.id);
      });
      item.append(button);
    }
    missionList.append(item);
  }
}

async function refreshMissions(): Promise<void> {
  if (!session) return;
  try {
    renderMissions(await loadMissions(session));
    if (missionsErr) missionsErr.textContent = "";
  } catch (err) {
    if (missionsErr) missionsErr.textContent = err instanceof Error ? err.message : "任务读取失败";
  }
}

function showFriends(): void {
  view = "friends";
  if (home) home.hidden = true;
  if (profile) profile.hidden = true;
  if (missions) missions.hidden = true;
  if (guild) guild.hidden = true;
  if (play) play.hidden = true;
  if (friendsNote) friendsNote.textContent = "";
  if (friendsErr) friendsErr.textContent = "";
  if (friends) friends.hidden = false;
  void refreshFriends();
}

function renderFriendGroup(list: HTMLUListElement | null, rows: FriendPerson[] | FriendRequestRow[], empty: string, actions: (row: FriendPerson) => Array<[string, () => void]>): void {
  if (!list) return;
  list.replaceChildren();
  if (rows.length === 0) {
    const item = document.createElement("li");
    item.textContent = empty;
    list.append(item);
    return;
  }
  for (const row of rows) {
    const item = document.createElement("li");
    const text = document.createElement("span");
    const time = "time" in row ? row.time : 0;
    text.textContent = friendLine(row, time);
    item.append(text);
    const buttons = actions(row);
    if (buttons.length > 0) {
      const bar = document.createElement("span");
      bar.className = "row-actions";
      for (const [label, run] of buttons) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = label;
        button.disabled = friendBusy;
        button.addEventListener("click", () => {
          run();
        });
        bar.append(button);
      }
      item.append(bar);
    }
    list.append(item);
  }
}

function renderFriends(card: FriendList): void {
  renderFriendGroup(friendList, card.friends, "暂无好友", (row) => [["删除", () => void changeFriend("删除", () => deleteFriend(session!, row.roleid))]]);
  renderFriendGroup(friendIncoming, card.incoming, "暂无申请", (row) => [
    ["同意", () => void changeFriend("同意", () => agreeFriend(session!, row.roleid))],
    ["拒绝", () => void changeFriend("拒绝", () => rejectFriend(session!, row.roleid))],
  ]);
  renderFriendGroup(friendOutgoing, card.outgoing, "暂无申请", () => []);
}

async function refreshFriends(): Promise<void> {
  if (!session) return;
  try {
    renderFriends(await loadFriends(session));
    if (friendsErr) friendsErr.textContent = "";
  } catch (err) {
    if (friendsErr) friendsErr.textContent = err instanceof Error ? err.message : "好友读取失败";
  }
}

function friendButtons(): HTMLButtonElement[] {
  if (!friends) return [];
  return [...friends.querySelectorAll("button")];
}

function lockFriendButtons(locked: boolean): void {
  friendBusy = locked;
  for (const button of friendButtons()) {
    if (button === friendsBack) continue;
    button.disabled = locked;
  }
}

async function changeFriend(label: string, run: () => Promise<void>): Promise<void> {
  if (!session) return;
  lockFriendButtons(true);
  try {
    await run();
    if (friendsNote) friendsNote.textContent = `${label}成功`;
    await refreshFriends();
  } catch (err) {
    if (friendsNote) friendsNote.textContent = "";
    if (friendsErr) friendsErr.textContent = err instanceof Error ? err.message : `${label}失败`;
  } finally {
    lockFriendButtons(false);
  }
}

async function sendFriendApply(): Promise<void> {
  if (!session || !friendIdInput) return;
  const roleid = friendIdInput.value.trim();
  if (!roleid) {
    if (friendsErr) friendsErr.textContent = "请填写角色号";
    return;
  }
  await changeFriend("申请", async () => {
    await applyFriend(session!, roleid);
    friendIdInput.value = "";
  });
}

function onFriendNotice(kind: string): void {
  if (view !== "friends") return;
  if (friendsNote) friendsNote.textContent = friendNoticeText(kind);
  void refreshFriends();
}

function showGuild(): void {
  view = "guild";
  if (home) home.hidden = true;
  if (profile) profile.hidden = true;
  if (missions) missions.hidden = true;
  if (friends) friends.hidden = true;
  if (play) play.hidden = true;
  if (guildNote) guildNote.textContent = "";
  if (guildErr) guildErr.textContent = "";
  guildHit = null;
  joinedGuild = false;
  if (guildQuery) guildQuery.value = "";
  renderGuildHit();
  if (guild) guild.hidden = false;
  void refreshGuild();
}

function renderMyGuild(mine: MyGuild | null): void {
  if (guildCreate) guildCreate.hidden = mine !== null;
  if (!guildCard) return;
  guildCard.replaceChildren();
  const title = document.createElement("h3");
  title.textContent = "我的公会";
  guildCard.append(title);
  if (!mine) {
    const empty = document.createElement("p");
    empty.className = "guild-empty";
    empty.textContent = "还没有公会";
    guildCard.append(empty);
    return;
  }
  guildCard.append(guildMeta(`${mine.name}　${mine.guildid}`));
  guildCard.append(guildMeta(mine.notice || "暂无公告"));
  const members = document.createElement("h3");
  members.textContent = "成员";
  guildCard.append(members);
  guildCard.append(guildPeople(mine.members, "暂无成员", (row) => {
    if (mine.leader !== roleID || row.roleid === roleID) return [];
    return [["踢出", () => void changeGuild("踢出", () => kickGuild(session!, row.roleid))]];
  }));
  if (mine.leader === roleID) {
    const applies = document.createElement("h3");
    applies.textContent = "申请";
    guildCard.append(applies);
    guildCard.append(guildPeople(mine.applies, "暂无申请", (row) => [
      ["同意", () => void changeGuild("同意", () => agreeGuild(session!, row.roleid))],
      ["拒绝", () => void changeGuild("拒绝", () => rejectGuild(session!, row.roleid))],
    ]));
  }
  const action = document.createElement("button");
  action.type = "button";
  action.disabled = guildBusy;
  if (mine.leader === roleID) {
    action.textContent = "解散";
    action.addEventListener("click", () => {
      void changeGuild("解散", () => disbandGuild(session!));
    });
  } else {
    action.textContent = "退出";
    action.addEventListener("click", () => {
      void changeGuild("退出", () => leaveGuild(session!));
    });
  }
  guildCard.append(action);
}

function renderCatalog(rows: GuildBrief[], inGuild: boolean): void {
  if (!guildCatalog) return;
  guildCatalog.replaceChildren();
  if (rows.length === 0) {
    const item = document.createElement("li");
    item.textContent = "暂无公会";
    guildCatalog.append(item);
    return;
  }
  for (const row of rows) guildCatalog.append(guildRow(row, inGuild));
}

function renderGuildHit(): void {
  if (!guildResult) return;
  guildResult.replaceChildren();
  if (!guildHit) return;
  guildResult.append(guildRow(guildHit, joinedGuild));
}

function guildRow(row: GuildBrief, inGuild: boolean): HTMLLIElement {
  const item = document.createElement("li");
  const text = document.createElement("span");
  text.textContent = catalogLine(row);
  item.append(text);
  if (!inGuild) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "申请";
    button.disabled = guildBusy;
    button.addEventListener("click", () => {
      void changeGuild("申请", () => applyGuild(session!, row.guildid));
    });
    item.append(button);
  }
  return item;
}

function guildMeta(text: string): HTMLParagraphElement {
  const line = document.createElement("p");
  line.className = "guild-meta";
  line.textContent = text;
  return line;
}

function guildPeople(rows: Array<GuildMember | GuildApply>, empty: string, actions: (row: GuildMember | GuildApply) => Array<[string, () => void]>): HTMLUListElement {
  const list = document.createElement("ul");
  if (rows.length === 0) {
    const item = document.createElement("li");
    item.textContent = empty;
    list.append(item);
    return list;
  }
  for (const row of rows) {
    const item = document.createElement("li");
    const text = document.createElement("span");
    text.textContent = memberLine(row, "rank" in row ? row.rank : undefined);
    item.append(text);
    const buttons = actions(row);
    if (buttons.length > 0) {
      const bar = document.createElement("span");
      bar.className = "row-actions";
      for (const [label, run] of buttons) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = label;
        button.disabled = guildBusy;
        button.addEventListener("click", () => {
          run();
        });
        bar.append(button);
      }
      item.append(bar);
    }
    list.append(item);
  }
  return list;
}

async function refreshGuild(): Promise<void> {
  if (!session) return;
  try {
    const [mine, catalog] = await Promise.all([loadMyGuild(session), loadGuildCatalog(session)]);
    joinedGuild = mine !== null;
    renderMyGuild(mine);
    renderCatalog(catalog, joinedGuild);
    renderGuildHit();
    if (guildErr) guildErr.textContent = "";
  } catch (err) {
    if (guildErr) guildErr.textContent = err instanceof Error ? err.message : "公会读取失败";
  }
}

function lockGuildButtons(locked: boolean): void {
  guildBusy = locked;
  if (!guild) return;
  for (const button of guild.querySelectorAll("button")) {
    if (button === guildBack) continue;
    button.disabled = locked;
  }
}

async function changeGuild(label: string, run: () => Promise<void>): Promise<void> {
  if (!session) return;
  lockGuildButtons(true);
  try {
    await run();
    if (guildNote) guildNote.textContent = `${label}成功`;
    await refreshGuild();
  } catch (err) {
    if (guildErr) guildErr.textContent = err instanceof Error ? err.message : `${label}失败`;
  } finally {
    lockGuildButtons(false);
  }
}

async function sendGuildFind(by: "id" | "name"): Promise<void> {
  if (!session || !guildQuery) return;
  const value = guildQuery.value.trim();
  if (!value) {
    if (guildErr) guildErr.textContent = by === "id" ? "请填写公会编号" : "请填写公会名";
    return;
  }
  lockGuildButtons(true);
  try {
    guildHit = by === "id" ? await findGuildByID(session, value) : await findGuildByName(session, value);
    renderGuildHit();
    if (guildErr) guildErr.textContent = "";
    if (guildNote) guildNote.textContent = "";
  } catch (err) {
    guildHit = null;
    renderGuildHit();
    if (guildErr) guildErr.textContent = err instanceof Error ? err.message : "查找失败";
  } finally {
    lockGuildButtons(false);
  }
}

async function sendGuildCreate(): Promise<void> {
  if (!session || !guildNameInput || !guildNoticeInput) return;
  const name = guildNameInput.value.trim();
  if (!name) {
    if (guildErr) guildErr.textContent = "请填写公会名";
    return;
  }
  const notice = guildNoticeInput.value.trim();
  await changeGuild("创建", async () => {
    await createGuild(session!, name, notice);
    guildNameInput.value = "";
    guildNoticeInput.value = "";
  });
}

function onGuildPush(kind: string): void {
  if (view !== "guild") return;
  if (guildNote) guildNote.textContent = guildNoticeText(kind);
  void refreshGuild();
}

async function claim(id: number): Promise<void> {
  if (!session || !missionList) return;
  for (const button of missionList.querySelectorAll("button")) button.disabled = true;
  try {
    const result = await claimMission(session, id);
    renderMissions(result.list);
    const reward = result.reward > 0 ? `，奖励等级 +${result.reward}` : "";
    if (missionsNote) missionsNote.textContent = `领取成功${reward}，当前等级 ${result.level}`;
    if (missionsErr) missionsErr.textContent = "";
  } catch (err) {
    for (const button of missionList.querySelectorAll("button")) button.disabled = false;
    if (missionsErr) missionsErr.textContent = err instanceof Error ? err.message : "领取失败";
  }
}

levelBtn?.addEventListener("click", () => {
  void changeLevel();
});

async function changeLevel(): Promise<void> {
  if (!session || !levelInput) return;
  const level = Number(levelInput.value);
  if (!Number.isInteger(level) || level < 1) {
    if (profileErr) profileErr.textContent = "等级至少为 1";
    return;
  }
  if (levelBtn) levelBtn.disabled = true;
  try {
    const next = await saveLevel(session, level);
    if (profileLevel) profileLevel.textContent = String(next);
    levelInput.value = String(next);
    if (profileErr) profileErr.textContent = "";
  } catch (err) {
    if (profileErr) profileErr.textContent = err instanceof Error ? err.message : "修改失败";
  } finally {
    if (levelBtn) levelBtn.disabled = false;
  }
}
