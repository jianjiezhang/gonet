# gonet

单进程 actor 内核。复制本目录即可在新项目里写业务，不要把玩法、数据库、玩家 TCP 协议放进来。

Go 模块名：`gonet`。业务侧可用 `replace gonet => ./gonet`。

## 对外 API

| 函数 | 作用 |
|------|------|
| `Spawn` / `SpawnNamed` / `SpawnWith` / `SpawnAsync` | 立刻入表并排队 `.init`；**不等 Init**。`starting` 时只收 init。`WaitInit` 可等就绪；`SpawnAsync`+`InitFrom` 用 `.response` 收 Init 结果 |
| `WaitInit` | 等到 Init 成功或失败 |
| `StopActor` / `StopActorWait` / `Kill` / `Stop` / `StopWait` | `StopActor` 一直等 Term；`StopActorWait`/`StopWait` 可带超时；`Kill`/`Exit` 只发停机 |
| `Send` / `SendName` | 业务消息先编成 JSON 再投递。包级函数 `From=0`；`ActorContext.Send` / `Envelope.Send` 带自身 pid |
| `SendMemory` / `SendMemoryName` | 原指针投递，不序列化。只在本进程。`Conn` 这类值用它 |
| `Call` / `CallName` | 异步请求，正文同样先编成 JSON。返回的 `error` 只表示这次是否入队；成功则带 `session`，结果以后到 `.response` |
| `CallMemory` / `CallMemoryName` | 异步请求，正文保持原指针 |
| `SuspendCall` / `SuspendCallName` | 阻塞当前协程等 `Reply`。给 main / 测试用，**禁止在 Dispatch 里调用** |
| `SuspendCallMemory` / `SuspendCallMemoryName` | 阻塞 Call，正文保持原指针 |
| `Register` / `Unregister` / `Query` | 别名 |
| `RegisterCmd` / `Pack` | cmd 工厂 + `{cmd, data}` JSON；`.` 前缀系统命令不能 Pack |
| `Timeout` / `StopTimer` | 到期后 `Send` 到目标 mailbox |
| `Monitor` | 只读快照（全局 + 每 actor 的 full/dead/call_timeout/slow/high） |
| `ListenDebug` / `StopDebug` | 行协议：`monitor` / `kill <pid>` / `help` / `quit` |

业务通过嵌入 `ActorContext` 实现 `Init` / `Dispatch` / `Term`。`Init`/`Dispatch` 里禁止 `Accept`、`Read`、`SuspendCall` 或其它会堵住 mailbox 的调用。网络读放独立协程，再 `Send` 回自己。

## 复制后最少写法

```go
pid, _ := gonet.SpawnNamed(impl, ".app")
_, _ = gonet.ListenDebug("127.0.0.1:9999")
gonet.Stop() // 已包含 StopDebug
```

第一个服务可以自己 `SpawnNamed`，不必带游戏服的 launcher。

## 约定

**别名。** 一对一，actor 死后自动摘名。名字不在本进程时返回 `ErrUnknownAlias`。

**消息。** `Send`/`Call` 要求消息带有非空 cmd，并把结构体 `json.Marshal` 成 `*BaseMessage` 再入队；`Dispatch` 按 cmd 工厂 `Unpack` 成新结构体。已经是 `*BaseMessage`（例如 `Pack`）则原样投递。系统命令（`.` 前缀）不进 JSON。`SendMemory`/`CallMemory` 跳过编码，handler 拿到原指针。`net.Conn`、actor 实现走内存接口。回复值仍是原来的 Go 值，不进 JSON。

异步 `Call` 的调用方应 `RegisterCmd(gonet.CmdResponse, ...)` 处理 `CallResponse`。未登记时默认静默丢弃（不报未知命令）。

**定时器。** 一次性。需要周期就在 handler 里再 `Timeout`。actor 停止时未触发的 timer 会被取消。到期只 `Send`。mailbox 满则打错误日志，不重试。

**Debug。** 建议绑 `127.0.0.1`。`monitor` 只读；`kill` 会停 actor。

## 停机与邮箱

`StopActor` / `Stop` 关闭 `quit` 后进入 `Term`。**mailbox 里尚未 `Dispatch` 的消息会被丢掉**（异步 Call 会尽量回 `ErrDead`）。别名在 `done` 关闭前已从进程表摘掉。

- 持久化放在 `Term`（不要指望队列里最后一条 persist 一定跑到）。
- 停自己用 `Exit()` / `Kill(pid)`，不要同步 `StopActor(Self())`。
- `StopActorWait` / `StopWait` 超时返回后对方可能还在 `Term`，只是调用方不再死等。

`Spawn` 立刻返回 pid，Init 是 mailbox 里的第一条系统消息（`.init`）。在 `Init` 成功前状态为 `starting`，其它 `Send`/`Call` 返回 `ErrNotReady`。

- `WaitInit(ctx, pid)`：main / 测试等就绪。
- `SpawnAsync(..., InitFrom, InitTimeout)`：Init 当成对创建方的异步 Call。`InitTimeout>0` 才会超时通知创建方，**不 Kill、不摘名**。`InitTimeout<=0` 只异步等 Init 结束，不发超时。actor 仍在表里（`starting` 时 Send 为 `ErrNotReady`），`Query`/`Monitor` 找得到。超时后晚到的 Init 成功会变成 `running`，成功 `Reply` 因 session 已作废而丢掉。要清掉需显式 `Kill`/`StopActor`，且要等 `Init` 返回后 Term 才算 Kill 成功。
- Init 失败：actor 摘表；`WaitInit` / `.response` 得到错误。

`SpawnOptions.InitTimeout` 仅在 `InitFrom!=0` 时有效。

## Send / Call 的返回值

`error` **只表示有没有进入对方 mailbox**，不表示对方已处理完。

| 返回 | 含义 |
|------|------|
| `nil` | 已入队。`Call` 同时返回 `session` |
| `ErrDead` / `ErrUnknownAlias` | 没发出去，不会有 `CallResponse` |
| `ErrMailboxFull` | 没发出去，可稍后重试；不会有 `CallResponse` |
| `ErrNoCaller` / `ErrNilMessage` | 参数不合法 |

入队成功后，**通常**会有一条 `.response`（成功、超时、对方死亡、或 Dispatch panic）。若调用方 mailbox 满导致回复投递失败，只打错误日志并记 `reply_fail`，**不会再有结果**——业务只能靠自己的 Call 超时当兜底（超时消息同样可能投失败）。不要把文档理解成「百分之百必达」。

RPC 成败只看 `CallResponse`，不要看 `Call` 的 `error`。

## Call 超时

`d>0` 才会排超时；`d<=0` 只等 `.response`，不发 `ErrCallTimeout`。

- `Err == nil`：`Value` 是对方 `Reply`
- `ErrCallTimeout`：到点未 `Reply`（仅 `d>0`）
- `ErrDead`：对方退出时请求还在队列里
- `ErrDispatchPanic`：对方 Dispatch panic

超时 **不会撤回** 对方队列里的请求；晚到的 `Reply` 丢掉。「超时 ≠ 没执行」，重试必须幂等。

`SuspendCall` 阻塞当前协程，只给邮箱外面的协程用。邮箱协程里用异步 `Call`，对自己也一样。

## Monitor 指标

全局：`send_full` / `send_dead` / `call_timeouts` / `slow_dispatch`（单次 Dispatch ≥ 100ms）/ `reply_fail`。

每 actor：mailbox 深度与 **high** 水位、`pending_calls`、各自的 full/dead/call_to/slow/reply_fail、`last_cmd`、`last_dispatch`。

`gonet/mysql` 是无玩法的连接池 + SQL + `Codec`（JSON / gzip+JSON）。表名、主键、结构体由业务提供。不要在 `Dispatch` 里跑 SQL。

`gonet/redis` 是无玩法的连接池（string / hash / expire）。`Open` 传入地址、用户、密码、库号。不要在 `Dispatch` 里访问 Redis。

## 不要放进本仓库的

登录、任务、玩家表结构、玩家包格式。那些是业务。

进程内拉起其它 actor 走 `services/launcher`，和 harbor 一样放在本模块，不放进玩法包。
