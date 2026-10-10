# 游戏服当前设计指引

面向后续开发者和 AI。改代码前先读本文，再对照源码。本文描述的是**当前已落地的行为**，不是远期规划。

模块：`game`（根目录 `go.mod`），Go 1.22。内核 `gonet` 用 `replace gonet => ./gonet` 指向本仓库子目录。风格对齐 Skynet：服务是 actor，用名字寻址，创建走 launcher。gonet 自身说明见 `gonet/README.md`。

---

## 1. 现在做成了什么

进程内 actor 运行时。固定服务见下表；每个公会另外一个 `guild/<id>`。

| 服务 | 别名 | 职责 |
|------|------|------|
| launcher | `.launcher_<nodeid>` | 进程内第一个服务；唯一「业务创建 actor」入口 |
| onlinemgr | `.onlinemgr_<nodeid>` | 本节点在线名单；登录/下线同步、是否在线、批量查询、广播 |
| friend | `.friend` | 全服好友关系和未处理申请。上限 50。不存名字和在线 |
| guildmgr | `.guildmgr` | 公会目录：创建、一人一公会、拉起 `guild/<id>`。不管成员细节 |
| match | `.match` | 房间目录：创建、加入、离开、人满。开场才拉起 `room/<id>` |
| idgen | `.idgen` | 全服编号。只在 `center = true` 的进程启动。按种类各自递增，种类名由调用方传入 |
| watchdog | `.watchdog_<nodeid>` | 接入：listen 协程、login（token）、踢号、按 roleid 建 role |
| role | `role/<roleid>` | 玩家会话：基础数据、任务模块、TCP、心跳/存盘用 `Timeout` |
| guild | `guild/<公会id>` | 一个公会一个 actor：成员、申请、公告。由 guildmgr 创建 |
| room | `room/<房间号>` | 一场已开场的场景。模式 4 有帧循环和目标判定；其它模式仍是空结算 |

另有：

- `config/config.toml`：玩家口、debug 口、`login_token`、MySQL、Redis
- `lib/packet`：TCP 长度前缀帧
- `lib/net`：客户端 `Codec`（默认 JSON），`ReadMsg`/`WriteMsg`
- `store`：MySQL 连接池，表 `role`、`t_mission`、好友和公会；不是 actor
- `cmd/client`：登录后可打任务、好友、公会，以及 `roomcreate` / `roomjoin` / `roomleave` / `roomstart` / `roomsettle`
- `cmd/debugclient`：连 `127.0.0.1:9999`，行协议 `monitor` / `kill`
- `web`：提供 `clientgame/dist`。`/api/login` 打一帧登录后关掉。`/api/session` 把浏览器 WebSocket 上的同一帧转到玩家口，连接保持。登录后是主页，再进个人资料、任务或玩法选关。任务走 `missionlist` / `missionfinish`。对局仍在浏览器里算。

**没有做**：独立 gate 进程、heros/partys、跨服。

---

## 2. 目录

```text
project/
  main.go                 # 配置、store、SpawnNamed launcher、NewService watchdog、ListenDebug
  web/                    # 页面、/api/login、/api/session
  config/                 # config.toml、任务静态表
  lib/
    packet/               # 4 字节长度 + 2 字节命令号 + data
    net/                  # 帧的收发和 data 的 JSON；不带具体命令号
  protocol/               # 玩家 TCP（独立 go.mod）
    client/               # cmd.go 命令号，msg.go 正文
  store/                  # Store：role 行 + t_mission JSON blob
  cmd/client/             # 玩家客户端
  cmd/debugclient/        # debug 口客户端
  gonet/                  # actor 内核（独立 go.mod）
    api.go / actor / timer / monitor / mysql / redis / README.md
    services/
      launcher/           # 进程内创建入口
  service/
    watchdog/
    onlinemgr/            # 在线名单、是否在线、广播
    friend/               # 好友边、申请、人数上限
    guildmgr/             # 公会目录：创建、一人一公会、拉起公会 actor
    guild/                # 一个公会一个 actor：成员、申请、公告
    match/                # 房间目录：等人。开场才拉起场景
    idgen/                # 全服编号。只在 center 启动。角色、公会、房间还没改来这里取号
    room/                 # 一场已开场的空场景
    role/                 # BaseData、Missions、persist、TCP cmd
```

`test/游戏服务器主程学习计划.md` 是个人学习笔记，**不是本服设计文档**。

---

## 3. 启动顺序

必须在项目根运行，才能读到 `config/config.toml`。

```text
main
  config.Load
  store.Open（装上本进程唯一的 store，actor 用 store.Get 取）
  gonet.SpawnNamed(launcher, ".launcher")
  gonet.WaitInit(launcher)
  center = true 时 launcher.WaitService(idgen)  # 全服编号。其它节点不启动
  launcher.WaitService(onlinemgr)              # 先于 watchdog，role 上线时它已经在
  launcher.WaitService(friend)                 # Init 时装载全服好友和申请
  launcher.WaitService(guildmgr)               # 装载公会目录，并拉起已有的 guild/<id>
  launcher.WaitService(match)                  # 房间目录。场景 actor 开场才有
  launcher.WaitService(watchdog)               # Init 完成后再返回；Init 里 StartAccept
  gonet.ListenDebug(debug_host:debug_port)    # debug_port=0 则跳过
  web.Start(web_host:web_port)                 # 页面、/api/login、/api/session；web_port=0 则跳过
  等待 SIGINT/SIGTERM
  StopActor(watchdog) → StopActor(match) → StopActor(friend) → StopActor(guildmgr) → StopActor(idgen) → StopActor(onlinemgr) → StopActor(launcher) → gonet.Stop（含 StopDebug）
```

业务服务（含 watchdog、role）**禁止**自己调 `gonet.Spawn` / `SpawnNamed`。一律：

```go
launcher.NewService(fromPID, impl, serviceName, timeout, callback) // 异步；完成在调用方 mailbox 跑 callback
launcher.WaitService(ctx, impl, serviceName)                       // 阻塞；仅 main / 测试
```

`NewService` 向 `.launcher` 发 **Call**（cmd `newservice`，带 `Impl` 指针，不能 Pack）。launcher 再 `SpawnAsync`，`InitFrom` 为自己、`InitTimeout=0`（只等 Init，不因超时失败）。`timeout` 只约束这次 Call 何时给调用方 `.response`。调用方 `RegisterCmd(.response)` 后先 `launcher.DispatchResponse`，匹配到则跑 `callback(ServiceResult)`。`serviceName` 是新服务别名；空字符串表示匿名。

---

## 4. gonet 内核（必须遵守）

细节以 `gonet/README.md` 为准。业务侧记住：

- 玩家 TCP 的命令号在 `protocol/client/cmd.go`，正文在 `protocol/client/msg.go`。服务侧消息结构和登记都在 `ActorMessage.go`，包括客户端发来的，命令名用 `protocol/client` 里的那一个。`RegisterCmds()` 在 `Init` 里调用。launcher 无客户端协议。
- 一个 actor 一个 mailbox，**Dispatch 里禁止 Accept/Read/堵网**。
- mailbox 协程上的 info、warn、error 前面带这个 actor 的别名，例如 `[role/39000001]`。玩家协议写出前再加 `[send]`，读入后加 `[recv]`，后面是命令和 JSON。readLoop 和等登录不在 mailbox 上，进入时带上同一个别名。
- `Spawn` 立刻返回；Init 是 mailbox 第一条系统消息。`starting` 时只收 init，其它投递得 `ErrNotReady`。就绪用 `WaitInit` 或 `SpawnAsync(InitFrom)`；`InitTimeout<=0` 只等 Init，不超时失败。
- `Send`/`Call` 走 pid；`SendName`/`CallName` 走别名。入队前把业务消息编成 JSON（`*BaseMessage`），`Dispatch` 再按 cmd `Unpack`。`Call` 是异步的，回复走 `.response`（回复值仍是原来的 Go 值）。`d<=0` 不排超时。Dispatch 里用异步 `Call`，不要 `SuspendCall`。
- `SendMemory`/`SendMemoryName`（以及 `CallMemory`）不编码，原指针只在本进程传递。`Conn`、创建 actor 的 `Impl` 走这条。`.` 开头的内核消息（`.init`、`.response`、`.call.timeout`）也不进 JSON。
- 停机可用 `StopActorWait` / `StopWait` 带超时。
- `Monitor` 含每 actor 的 mailbox high、投递失败与 Call 超时计数。
- 周期逻辑用 `Timeout` 再续一次，不要旁路 ticker（role 心跳检查、30 秒存盘已如此）。
- 停自己用 `Exit()`/`Kill`，不要在 Dispatch 里同步 `StopActor(Self())`。
- 已经 `Pack` 过的 `*BaseMessage` 会原样入队。`net.Conn`、actor 实现不要 `Pack`，也不要走 `Send`，用 `SendMemory` / `CallMemory`。
- `RegisterCmd`：同 cmd 再注册会覆盖全局工厂，类型变了打日志；handler 仍按 actor 各绑各的。

创建：`Spawn` / `SpawnNamed` / `SpawnWith` 是内核入口。业务只走 launcher（bootstrap launcher 除外）。

---

## 5. 接入与登录（watchdog）

- **独立协程** `StartAccept`：`Listen` + `Accept`。Init 启动，Term 里 StopAccept。
- mailbox：ping、socket.open、login、socket.close。

```text
Accept
  → pending >= 1024？直接关
  → 5 秒内必须收到 login（roleid + 可选 token）
  → config.login_token 非空则必须一致，否则回 login {err:auth} 并关连接
  → 库中没有：Create 默认号再继续
  → 已在线：异步 Call 旧 role 发 kick，回复后再在独立协程 StopActor，完成后建新号
  → NewService(role, 别名=roleid, callback) → Init 完成后 callback 里 SendMemory 把 conn 交给 role
  → 回 login ok；role 在 conn.attach 里启动 readLoop
```

断线：readLoop 退出 → `socket.close`（`alias` + 这次接入的 `link`）→ watchdog 内存里的 `link` 相同才 `StopActor`。顶号后旧连接的关闭或按别名投来的包对不上新 `link`，直接丢掉。若 role 已 `Exit`（心跳超时），`StopActor` 得到 `ErrDead`，记成断开即可，不当故障。本端已关连接的 Read/Write 不再打 `use of closed network connection`。

---

## 6. TCP 协议（lib/packet + lib/net）

玩家连接的帧在 `lib/packet`，收发在 `lib/net`。命令名、命令号、正文结构和组包在 `protocol/client`。业务用 `New*` 得到一条 `Out`，再 `Send` 给自己的写出函数，用 `DecodeReq` / `DecodeRsp` 解开正文，不手写 `map`。role 和玩家客户端启动时把命令号登记进 `lib/net`。`lib` 里没有具体玩法命令：

```text
[4 字节大端 uint32 = 后面的字节数 N][2 字节大端命令号][data]
```

`N` 含命令号和 data，不含长度自己。`N<2`、命令号为 0，或 `N>64KiB` 失败。data 默认是 payload 的 JSON；没有 payload 时 data 为空，不写 `null`。服务内部仍用命令名，写出时换成命令号。`ReadMsg` 按命令名 Unpack；本进程没登记该名字时仍是信封（测试客户端）。命令号不认识或 data 解不开是 `ErrCodec`，连接不断；帧错误则断开。不要 `import "game/lib"`。watchdog/role 不要再 `gonet.Unpack` 客户端包。

| 号 | 方向 | 命令 | 含义 |
|----|------|------|------|
| 1 | C→S | `login` | `roleid`，以及 `token`（与配置 `login_token` 比） |
| 1 | S→C | `login` | `"ok"` 或 `{"err":"auth"}` |
| 2 | S→C | `kick` | `"duplicate"` |
| 3 / 4 | C→S / S→C | `ping` / `pong` | |
| 5 | C→S / S→C | `heartbeat` | 网页接入时由本进程每 5 秒写一帧，页面不再另发；超过 15 秒无心跳则踢 |
| 6 | C→S / S→C | `missionlist` | 当前已接任务。每条含进度、目标和领取时增加的等级 |
| 7 | C→S | `missionfinish` | `{id}` 领取 |
| 7 | S→C | `missionfinish` | 新列表、奖励等级增量、当前等级；或 `{err}` |
| 8 | C→S / S→C | `roleinfo` | roleid / level / name / gender |
| 9 | C→S | `setlevel` | `{level}` |
| 9 | S→C | `setlevel` | 新等级 + 任务列表；或 `{err}` |
| 10 | C→S | `friendlist` | 好友、收到的申请、发出的申请 |
| 10 | S→C | `friendlist` | `friends` / `incoming` / `outgoing`，每项含 roleid、name、level、online；申请另有 time |
| 11–14 | C→S | `friendapply` / `friendagree` / `friendreject` / `frienddelete` | `{roleid}` |
| 11–14 | S→C | 同上 | `"ok"` 或 `{err}` |
| 15 | S→C | `friendnotify` | 对方发起的变化：`kind`（apply/agree/reject/delete）、`roleid`、`name`、`time` |
| 16–23 | C→S | `guildcreate` / `guildlist` / `guildapply` / `guildagree` / `guildreject` / `guildleave` / `guildkick` / `guilddisband` | 公会。创建带 `name`，申请带 `guildid`，同意、拒绝、踢人带 `roleid` |
| 24 | S→C | `guildnotify` | 公会变化：`kind`（`guild.apply` / `guild.agree` / `guild.reject` / `guild.leave` / `guild.kick` / `guild.disband`）、`guildid`、`roleid`、`name`、`time` |
| 25 | C→S / S→C | `guilds` | 当前全部公会。每条含 `guildid`、`name`、`notice`、`leader`、`leadername`、`level`、`online`、`members` |
| 26 | C→S | `guildid` | `{guildid}` 查一个公会 |
| 26 | S→C | `guildid` | 与列表里的一条相同；没有则 `{err}` |
| 27 | C→S | `guildname` | `{name}` 按公会名查 |
| 27 | S→C | `guildname` | 与 `guildid` 的回复相同 |
| 28 | C→S | `roomcreate` | `{mode, capacity}` 创建等人的房间 |
| 28 | S→C | `roomcreate` | 房间号、模式、人数、座位、`phase=wait`；或 `{err}` |
| 29 | C→S | `roomjoin` | `{roomid}` |
| 29 | S→C | `roomjoin` | 与创建成功相同；或 `{err}` |
| 30 | C→S | `roomleave` | 等人时离开目录；开场后离开场景 |
| 30 | S→C | `roomleave` | `"ok"` 或 `{err}` |
| 31 | C→S | `roomstart` | 人满才开场，拉起 `room/<id>` |
| 31 | S→C | `roomstart` | `phase=play`；或 `{err}` |
| 32 | C→S | `roomsettle` | 空结算。还没开场则拒绝 |
| 32 | S→C | `roomsettle` | `"ok"` 或 `{err}` |
| 33 | S→C | `roomnotify` | `kind`（join/leave/start/settle）、房间号、座位 |
| 34 | C→S | `roomop` | `{ax,ay,dash}` 对局操作。Ax/Ay 为 -1/0/1 |
| 35 | S→C | `roombegin` | 开局：种子、座位、区域、第一个目标、截止帧 |
| 36 | S→C | `roomframe` | 一帧：帧号、两座位操作、事件、玩家位置 |
| 37 | S→C | `roomresult` | `{win,reason,index,frame}` 对局结束 |
| 38 | C→S | `roomdead` | `{frame}` 这一帧被蛇咬到。帧号不能大于已广播的帧 |

进程内 ping 仍是 `gonet.SuspendCallName`（main 启动探测），不走 TCP。harbor 与 harbormgr 之间已是同一帧格式，命令号和结构在 `gonet/services/harbormgr/proto`，和玩家命令号各用各的。debug 口仍是行协议。

---

## 7. 玩家 role

- 别名 = `role/<roleid>`。登录用客户端带来的角色号。`NextAlias()` 给本进程新开的角色发号：`nodeid*1000000` 加本进程自增，接入路径目前不调用。
- `Init`：`LoadData`（role 行 + `t_mission`），`Base`/`Missions` bind dirty，Bootstrap 任务，注册 cmd，`Timeout` 心跳检查（5 秒）和 persist（30 秒）。超时关 conn 后 `Exit()`。连接由 watchdog `AttachConn`（`SendMemory`）送到 role 后再 `go readLoop`。接上连接时把 `lastlogintime`（unix 秒）写入资料并 `Send` 上线给 onlinemgr。
- `Term`：若这次会话宣布过在线，先写 `lastlogouttime` 并 `Send` 下线，再等异步存盘，然后同步 Save `role` 与 `t_mission`。
- onlinemgr 按 roleid 记在线。下线消息里的 pid 必须等于当前记录，避免旧会话的下线抹掉新登录。查询是 Call `online.query`，回复 bool。`online.broadcast.all` / `online.broadcast.some` 把客户端 cmd 和 JSON 文本转成 `online.push`，由 role 写出连接。`some` 只发给名单里仍在线的人。
- `BaseData` 字段不导出。`SetLevel` 会 `Notify("level")` 推进任务。
- **任务**：静态定义在 `config/mission.go`（等级链 1001→1002→1003）。进度整包 JSON 存在 `t_mission.data`。领取成功按 `RewardLevel` 调用 `AddLevel`。
- **Store**：进程里一份。`Open` 装上后，actor 用 `store.Get()` 取，不要在服务里再存一份地址。`HasRole`/`CreateRole`/`LoadRole`/`SaveRole`/`LoadMission`/`SaveMission`，以及好友边和申请。一次要改多张表的写进 `gonet/mysql` 的事务：建会（公会行 + 会长）、同意入会（成员 + 删申请）、解散（申请、成员、公会）、同意好友（双向边 + 删申请）、玩家存档（`role` + `t_mission`）。单条 SQL 已经覆盖的，例如一次插入两条好友边，不再包事务。连接走 `gonet/mysql`，表结构仍在本包。不是 actor。login 无号则 CreateRole。Dispatch 里不跑 SQL。

partys / heros：**未实现**。应挂在 `role.Data` 上，不要单独 actor。

---

## 8. 配置

```toml
host = "0.0.0.0"
port = 8888
debug_host = "127.0.0.1"
debug_port = 9999          # 0 = 不开 debug
login_token = "dev"        # 空 = 不校验 token
db_host = "127.0.0.1"
db_port = 3306
db_user = "game"
db_password = "123456"
db_name = "game"
redis_host = "127.0.0.1"
redis_port = 6379
redis_user = "default"    # 空 = default 用户
redis_password = ""       # 空 = 不校验密码
redis_db = 0
```

解析器只支持简单 `key = value`。Redis 连接用 `gonet/redis.Open`，传入 `RedisAddr()`、`RedisUser`、`RedisPassword`、`RedisDB`。启动流程尚未打开 Redis。

---

## 9. 怎么跑

```bash
cd <项目根>
go run .                              # 玩家口 8888，debug 9999
go run ./cmd/client -roleid 39000001  # 默认 token=dev
go run ./cmd/debugclient              # monitor / kill <pid>
```

客户端登录后可输入 `setlevel 2`，再 `missionfinish 1001`。同一 `roleid` 再连会先 `kick` 旧连接。

---

## 10. 硬约束（改代码时）

1. 不要在 actor 的 `Init`/`Dispatch` 里阻塞 Accept/Read/`SuspendCall`/`StopActor`（停别人等 Term 请丢到独立协程再 Send 回来）。
2. 不要绕过 launcher 创建业务 actor（bootstrap launcher 除外）。
3. 不要把 `net.Conn` / actor 实现 Pack 成 JSON。
4. 玩家 TCP 必须走 `lib/packet`：4 字节长度 + 2 字节命令号 + data。命令号定义在 `protocol/client`。服务里用 `gamenet.ReadMsg`/`WriteMsg`，不要自己拼帧，也不要在服务里直接解客户端包。不要把玩家命令号写进 `lib`。harbor 的线上协议留在 `gonet/services/harbormgr/proto`，gonet 不依赖外部 protocol。
5. 玩家模块数据挂在 role 上，不要为 mission/hero 再 Spawn 一个 actor。
6. watchdog 不长期读业务包；login 之后 conn 只属于对应 role。
7. 不要把玩法写进 `gonet/`。
8. 等人的房间只放在匹配目录里。`room/<id>` 只在开场后存在，不拿 `net.Conn`。目录的别名按第 9 条，应是 `.match`。
9. 别名带不带节点号，看这个服务是不是每个节点都有一份。每个节点都有的，用 `.名字_<nodeid>`，例如 harbor、launcher、onlinemgr、watchdog。全服只有一个 actor 的，用 `.名字`，不加 nodeid，例如 `.friend`、`.guildmgr`、`.match`、`.idgen`，以后的 `.chat` 也一样。`.idgen` 只在 center 上创建。`role/<roleid>`、`guild/<id>`、`room/<id>` 按实例起名，也不加 nodeid。调用方按这个全服别名去 Call，不要去叫 `.friend_<自己的节点号>`。

---

## 11. 空房间

`.match` 一直在。创建、加入、离开、是否人满都在它的 mailbox 里。一场还没开场时只是一条内存记录：房间号、模式、人数上限、座位。一个人同时只在一个房间。人数上限 1 到 8。进程停了，这些记录就没了，不进 MySQL。

人满并且有人 `roomstart` 之后，目录才用 launcher 拉起 `room/<房间号>`。房间号从创建起不变。场景把座位抄进去，不接受新的加入。场景里没有蛇，也没有帧。在座的人 `roomsettle` 后，场景通知各个 `role`，目录忘掉这场，然后场景 `Exit`。目录和两边的 `role` 还在。

开场前离开只改目录。开场后离开交给场景：走一个就放开这一个 roleid；人都走了，场景结束。玩家命令由自己的 `role` 转发。连接不到目录，也不到场景。

模式 4（两人同场信使）：人数必须是 2。开场后场景按 20 帧/秒推进。服务器只推两个玩家的位置，判定是否碰到当前目标、是否超时。目标共 10 个，每个限时 30 秒，碰到即换下一个，蛇速倍数每次 ×1.2。场上固定底部大出发安全区，以及场上两个小安全区；没有单独的庇护区。两人从出发区出生。两条蛇不在服务器上演算：两边客户端用开局种子和这一帧的玩家位置各自计算，规则与 `service/room` 的 `Chase` 相同，并与玩法 3 对齐——格子走、身体跟随、追猎物/闲逛、不进安全区。猎物进安全区后蛇失去目标去闲逛。被咬到的人上报 `roomdead`，该座位之后不再得分。两人都出局则 `roomresult` 的原因是 `bite`。网页主页「双人同场」已接创建/加入/开场与帧画面。
