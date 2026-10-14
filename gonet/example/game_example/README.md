# 反向贪吃蛇

gonet 的游戏例子。人是猎物，蛇来追人。登录、好友、公会、任务走游戏进程；单机三关在浏览器里算；双人同场由服务器推玩家和目标，两条蛇由两边按同一份数据各自演算。

在本目录执行下面的命令。配置、静态页和 `go run` 都相对这里。

## 需要什么

| 组件 | 用来做什么 | 怎么准备 |
|------|------------|----------|
| Go 1.22 | 游戏进程 | 本目录已有 `go.mod`，模块名 `game`，框架用 `replace gonet => ./gonet` |
| MySQL | 角色、好友、公会、任务 | 库要先建好。表不用手跑，进程起来时按 `store/game.sql` 自己建 |
| Redis | 公布 harbormgr 地址、节点号水位、发号水位 | 起来即可，不用预先写 key |
| Node.js 20 或 22 | 打出浏览器页面 | 只在 `clientgame` 里 `npm install` 和 `npm run build`。游戏进程不跑 Vite |

默认账号在 `conf/*.toml` 里：MySQL `game` / `123456`，库名 `game`，`127.0.0.1:3306`；Redis `127.0.0.1:6379`，用户 `default`，密码空，0 号库。

建库（表留给进程）：

```sql
CREATE DATABASE IF NOT EXISTS game DEFAULT CHARSET utf8mb4;
CREATE USER IF NOT EXISTS 'game'@'127.0.0.1' IDENTIFIED BY '123456';
GRANT ALL ON game.* TO 'game'@'127.0.0.1';
```

Redis 在本机直接起：

```bash
redis-server
```

页面要先打出来。游戏进程托管的是 `clientgame/dist`，目录不在会启动失败。

```bash
cd clientgame
npm install
npm run build
cd ..
```

`npm run dev` 只开 Vite，用来改 `clientgame/src` 时看页面。它也占 `127.0.0.1:5173`，和下面的登录页冲突，连游戏服时不要开着它。

## 启动和访问

### 单服

一个进程把中心、公共和玩家服务都打开，用 `conf/config.toml`。

```bash
go run . -config conf/config.toml
```

浏览器打开 `http://127.0.0.1:5173`。角色号随意填，第一次登录会建号。口令填 `dev`，对应配置里的 `login_token`。

本进程还听这些口：

| 口 | 地址 | 作用 |
|----|------|------|
| 登录页 | `127.0.0.1:5173` | 静态页，以及 `/api/session` 把浏览器转到玩家口 |
| 玩家口 | `0.0.0.0:8898` | watchdog 的 TCP。浏览器不直连，由网页转发 |
| debug | `127.0.0.1:9999` | monitor |
| harbormgr | 本机 IPv4 `:8008` | 全集群一份通讯录 |
| harbor | 本机 IPv4 `:8018` | 本进程收发室 |

`Ctrl+C` 停进程。停的时候先摘业务 actor，再关 harbor。

### 跨服

同一份程序，用三份配置拆成三个进程。开关决定本进程拉哪些 actor。harbormgr 只在 center 上起一份，把它的监听地址写入 Redis 键 `harbormgr:addr`。另外两个进程从 Redis 读这个地址，再连上去领节点号。节点号水位在 `harbormgr:nextnode`，发号水位在 `idgen:state`。

先起 center，再起 public，最后起 role。后两个会最多等 30 秒，直到 Redis 里有 harbormgr 地址。

```bash
# 终端 1：通讯录和 debug。不提供网页。
go run . -config conf/center.toml

# 终端 2：公会、发号、好友、匹配、在线表。
go run . -config conf/public.toml

# 终端 3：玩家接入和登录页。
go run . -config conf/role.toml
```

浏览器仍然打开 role 进程的页面：`http://127.0.0.1:5173`。登录方式和单服一样。

三个进程的 harbor 端口必须错开，都绑在本机一块非回环网卡的 IPv4 上：

| 进程 | 配置 | 打开的开关 | harbor | 其它口 |
|------|------|------------|--------|--------|
| center | `conf/center.toml` | `center` | 8018 | harbormgr 8008，debug `127.0.0.1:9999` |
| public | `conf/public.toml` | `guild` `idgen` `friend` `match` `onlinemgr` | 8019 | 玩家口 8897，无网页 |
| role | `conf/role.toml` | `role` | 8020 | 玩家口 8898，网页 `127.0.0.1:5173` |

再加一个玩家进程时，复制 `conf/role.toml`，改掉 `port`、`harbor_port`、`web_port`（例如 8899 / 8021 / 5174），等 public 起来后再启动。玩家从新的网页地址登录。房间、好友、公会、在线表仍然是全服那一份，不跟玩家进程走。

## 这个游戏

登录后是主页：个人资料、任务、好友、公会、单机玩法、双人同场。

单机玩法点「单机玩法」。对局在浏览器里跑，服务器不参与演算。

1. **猎人追逐**。WASD 或方向键移动，空格冲刺。蛇靠近会预警并扑咬，把蛇引进身体或边界可以断身。碰到蛇头即死。
2. **猎场信使**。3 分钟内按顺序站上信标 A、B、C。青绿色安全区能躲蛇头，但会从庇护变成不稳、瓦解，然后换位。场上有 4 个猎物机器人。
3. **全图信使**。从底部出发区开始，一次一个目标，碰满 5 个。安全区里蛇不追。

双人同场点「双人同场」，这是模式 4。一个人创建房间，另一个人填房间号加入，两人到齐后由房里的人点开始。服务器按 20 帧/秒推两个玩家的位置、当前目标和胜负；10 个目标，碰到一个就加快，超时或被咬则本场结束。蛇不在服务器上算。开场时服务器下发种子和安全区，两边用同一份玩家位置各自演算，画面上的蛇应当一致。

## 逻辑放在哪

```text
main.go                  读配置，打开 MySQL / Redis，按开关拉起 actor
conf/                    config.toml 单服；center / public / role 三进程
config/                  解析 toml。web_port = 0 表示本进程不提供网页
web/                     托管 clientgame/dist；/api/session 把 WebSocket 转到本机玩家口
protocol/client/         玩家命令号和消息体。浏览器 src/session.ts 与这里对齐
lib/packet、lib/net      帧和连接
store/                   表结构 game.sql，以及角色、好友、公会、任务的读写
cmd/client/              命令行客户端，直连玩家 TCP，不经过网页
```

浏览器界面在 `clientgame/`：

| 路径 | 内容 |
|------|------|
| `index.html` | 登录、主页、资料、任务、好友、公会、房间这些区块 |
| `src/main.ts` | 页面切换、键盘和登录 |
| `src/session.ts` | WebSocket 和命令号 |
| `src/profile.ts` `friends.ts` `guilds.ts` `missions.ts` | 主页上对应功能的请求 |
| `src/menu.ts` `game.ts` `draw.ts` | 玩法 1 |
| `src/missionGame.ts` `missionDraw.ts` `missionConfig.ts` | 玩法 2 和玩法 3 |
| `src/rooms.ts` `duoGame.ts` `duoDraw.ts` | 双人房间和画面 |
| `src/duoChase.ts` | 双人场两条蛇的浏览器演算 |

服务端 actor 在 `services/`。全服只有一份的用固定别名；每个进程一份或每个玩家一份的带节点或编号。

| 目录 | 何时拉起 | 别名 | 管什么 |
|------|----------|------|--------|
| `watchdog/` | `role` | `.watchdog_节点号` | 玩家 TCP、校验口令、没有角色就建号、为每个在线玩家拉起 role |
| `role/` | 登录时由 watchdog 拉起 | `role/角色号` | 这个玩家的连接、资料、任务，以及好友、公会、房间的入口 |
| `onlinemgr/` | `onlinemgr` | `.onlinemgr` | 全服在线表，把通知转到对应 role |
| `friend/` | `friend` | `.friend` | 好友关系和申请 |
| `guildmgr/` `guild/` | `guild` | `.guildmgr`，`guild/公会号` | 目录管创建和一人一会；每个公会一个 actor，管成员和申请 |
| `idgen/` | `idgen` | `.idgen` | 按种类发号，水位放 Redis |
| `match/` | `match` | `.match` | 房间目录：创建、加入、离开、开场 |
| `room/` | 开场时由 match 拉起 | `room/房间号` | 一场场景。`duo.go` 推玩家、目标和胜负；`snake.go` 是与浏览器同一套蛇演算，给测试和命令行客户端用，房间循环本身不算蛇 |

每个进程都会拉起 `launcher` 和自己的 `harbor`。`center = true` 的进程额外拉起 `harbormgr`；`debug_port` 不为 0 时再拉起 `monitor`。业务 actor 都交给本进程的 launcher 创建。跨进程找人用上面的别名，调用方不用区分对方在不在本进程。
