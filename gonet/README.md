# gonet

纯 actor 的跨服框架。这里不写玩法、玩家协议或数据库表，只给上层一个能加人、能按名字找人、能跨进程送信的骨架。

Go 模块名是 `gonet`。业务模块用 `replace gonet => ./gonet` 引用本目录。

## 简介

一个集群看成一家公司，一个进程看成一个部门，一个 actor 看成一名员工。

公司要管的是两件事：人在哪个部门，以及别人怎么找到他。部门内部有一份花名册，名字和人都在本进程。公司另有一份总通讯录，记下某个名字目前登记在哪个部门。`harbormgr` 保管总通讯录，每个部门的 `harbor` 负责把本部门的名字报上去，并把寄往外部门的信送出去。

员工之间不记对方住在哪台机器。本地和跨服用同一套名字：`SendName` / `CallName`。名字在本进程，信就进本地邮箱；不在，就交给 harbor，按通讯录转到那个部门，再投进对方邮箱。调用方不用先判断「是不是跨服」。

人员分配不由员工自己招人。本部门谁可以新开一个 actor，由 `launcher` 统一办。这样创建、登记名字、等待 `Init` 完成，都走同一条路径。

名字由各业务自己规定。建议每个服都有一份的 actor 用 `服务名_节点号`，例如 `guild_12`；全服只有一份的 actor 直接用服务名，例如 `center`。框架自带的 launcher、harbor 走 `ServiceAlias`，拼出来是 `.服务名_节点号`。节点号还没有时，`ServiceAlias` 返回空字符串。

## 使用方式

gonet 不读配置文件。进程听哪个地址、先拉起谁、哪些服务要开，由外面的启动代码决定。游戏服里的 `conf/config.toml` 是业务进程的开关和端口，读完之后再调用下面这些入口。

和框架有关的启动参数就这几样：

- harbormgr 的监听地址，以及每个节点 harbor 要公布的本机地址。
- harbor 拿到节点号之后，`SetClusterNode`，再按业务自己的规则登记别名。每个服都有一份的服务可以用 `ServiceAlias`；自带的 launcher 就是这样 `Bind` 的。
- `SpawnOptions`：`Name` 是别名，`Mailbox` 是邮箱容量（默认 1024），`InitFrom` 把 `Init` 的结果回给创建方。

先把 harbor 拉起来并装上跨服钩子，再 `Spawn` 需要对外可见的 actor。钩子装上之前登记的名字只在本进程有效，不会自动补登到通讯录。

### 一个最小的 actor

业务类型嵌入 `ActorContext`，在 `Init` 里登记命令。`SpawnNamed` 返回时 `Init` 还在邮箱里排队，要等 `WaitInit`。邮箱协程里不要 `Accept`、不要阻塞读、不要 `SuspendCall`；`SuspendCall` 只给 `main` 和测试用。

```go
type pingMsg struct{ gonet.BaseMessage }

type echo struct{ gonet.ActorContext }

func (a *echo) Init() error {
	return a.RegisterCmd("ping", func() gonet.MessageInterface { return &pingMsg{} }, a.onPing)
}

func (a *echo) onPing(e gonet.Envelope) { e.Reply("pong") }

func main() {
	ctx := context.Background()
	pid, err := gonet.SpawnNamed(&echo{}, "echo")
	if err != nil {
		log.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, pid); err != nil {
		log.Fatal(err)
	}
	msg := &pingMsg{}
	msg.SetCmd("ping")
	v, err := gonet.SuspendCallName(ctx, "echo", msg)
	fmt.Println(v, err) // pong
	gonet.Stop()
}
```

`Send` / `Call` 会把结构体编成 JSON 再投递，双方各有一份。只有发送方此后不再碰的指针（连接、待创建的 actor 实现）才用 `SendMemory` / `CallMemory`，而且不能跨服。

### 单服：部门自己管人

没有别的进程时，不必启动 harbor。选定一个本进程节点号，`launcher.Bind` 会把它自己登记成 `.launcher_节点号`，之后新 actor 都请它创建。业务 actor 的名字按上面的建议另定。`WaitService` 会堵到对方 `Init` 结束，只能在 `main` 或测试里用。部门内部互相找人，仍然 `CallName`。

```go
ctx := context.Background()
launcher.Bind(1)
pid, err := gonet.SpawnNamed(launcher.New(), launcher.Name)
if err != nil {
	log.Fatal(err)
}
if err := gonet.WaitInit(ctx, pid); err != nil {
	log.Fatal(err)
}
if _, err := launcher.WaitService(ctx, &echo{}, "echo"); err != nil {
	log.Fatal(err)
}
```

已经在跑的 actor 要再拉起别人时，用 `launcher.NewService`。它是异步 `Call`，结果进调用方邮箱的 `.response`，在处理函数里先交给 `launcher.DispatchResponse`。

### 扩成跨服：同一套名字

只有一个进程启动 harbormgr，作为公司的总通讯录。每个进程再启动自己的 harbor，连上这个地址，等它发下节点号，然后才 `Bind`、才 `Spawn`。找别的部门时用对方实际登记的名字，调用写法和单服一样；名字不在本进程时由 harbor 转出去。下面找的是对方 launcher，它的名字来自 `ServiceAlias`。业务 actor 换成自己的 `服务名_节点号` 或全服唯一的服务名即可。

```go
ctx := context.Background()

// 公司进程，全集群只起一个
mgrAddr, err := harbormgr.Start(ctx, "127.0.0.1:17000")

// 每个部门进程。mgrAddr 是上面 harbormgr 的监听地址
addr, err := harbor.Start(ctx, "127.0.0.1:0", mgrAddr)
nodeID, err := harbor.WaitReady(ctx)
gonet.SetClusterNode(nodeID)
launcher.Bind(nodeID)
pid, err := gonet.SpawnNamed(launcher.New(), launcher.Name)
err = gonet.WaitInit(ctx, pid)

// 任意一个已经有别名的 actor，按名字找另一个部门的 launcher
other := gonet.ServiceAlias("launcher", otherNodeID)
msg, err := gonet.Pack("ping", struct{}{})
_, err = gonet.CallName(fromPID, other, time.Second, msg)
```

`fromPID` 必须是一个活着的 actor，而且这个 actor 自己有别名，回复才送得回来。名字不在本进程时，`CallName` 会等到对方邮箱收下才返回；收不进去就返回错误，不会再有 `.response`。`addr` 是本部门 harbor 实际监听的地址，给别的节点连过来用。

## 深入了解

### 骨架

```text
gonet/
  api.go                     对外入口，业务只引这个包
  actor/                     一名员工：邮箱、生命周期、按名字投递、Call
  timer/                     一次性定时器，到期后给目标 actor 发一条消息
  services/
    launcher/                本部门的人员分配
    harbormgr/               公司总通讯录：名字在哪个节点
    harbor/                  本部门的收发室：登记名字、把信送到别的节点
    monitor/                 只读快照和 debug 口
  components/mysql、redis/   连接池，没有玩法，也不在邮箱协程里调用
```

一名员工是一条协程加一个有界邮箱。`Spawn` 立刻返回，`.init` 是邮箱里的第一条消息。`Init` 成功后才是 `running`，在此之前别人来信会得到 `ErrNotReady`。停机先关 `quit`，当前这条消息处理完，下一轮 `recv` 再执行 `Term`。

信分两层。邮箱里的 `Envelope` 记录谁发出的、怎么回。正文是业务结构体，发送时编成 `BaseMessage`（命令名加 JSON）。每个 actor 自己有一张命令表：同一个命令名在两个 actor 上可以对应不同的结构体。以 `.` 开头的命令留给内核，例如 `.init`、`.response`、`.call.timeout`，业务不能 `Pack` 它们。

异步 `Call` 的调用方必须自己是 actor。请求入队后马上返回 `session`，结果以后以 `.response` 进调用方邮箱。超时由定时器把一封 `.call.timeout` 送回调用方；若这次调用还在等，内核把它改写成 `CallResponse`。对方先 `Reply` 了，这封超时信就丢掉。

### 职责划分

| 部分 | 管什么 | 不管什么 |
|------|--------|----------|
| `actor` | 一个人怎么收信、回信、初始化、停机 | 这个人该不该被创建，信要送到哪台机器 |
| 进程内花名册 | 本部门的 pid 和别名，一对一 | 别的进程里有谁 |
| `launcher` | 本部门新增 actor，并等他 `Init` 完 | 玩法规则、跨进程转发 |
| `harbormgr` | 名字登记在哪个节点，节点还活着没有 | 消息正文、业务 actor 的生命周期 |
| `harbor` | 把本部门名字报给通讯录；本地没有的名字转出去；把对方的回复送回来 | 决定业务上该创建谁 |
| `monitor` | 邮箱水位、投递失败、慢处理和 debug 行协议 | 改通信路径 |
| `mysql` / `redis` | 连接和通用读写 | 表结构、键的含义；不要在 `Dispatch` 里直接访问 |

业务服务表达自己的工作方式是：嵌入 `ActorContext`，在 `Init` 里 `RegisterCmd`，用 `SendName` / `CallName` 找别的名字，在 `Term` 里释放自己的资源。对外名字由业务规定。每个服都有一份的，建议 `服务名_节点号`；全服唯一的，建议直接用服务名。玩家、房间这种大量短命的 actor 同样按业务自己的名字 `Register` 进花名册，例如 `role/39000001`；要被别的进程找到，就等 harbor 已经启动后再登记。

通信时记住这几条：

- `Send` / `Call` 返回的 `error` 只表示有没有进对方邮箱。处理成功与否看 `.response` 里的 `CallResponse`。
- 超时不会把已经入队的请求撤回来。对方晚到的 `Reply` 会被丢掉，重试必须能重复执行。
- 邮箱协程里只用异步 `Call`，包括调用自己。`SuspendCall` 会堵住这条协程，邮箱就不再收人。
- 停自己用 `Exit` 或 `Kill`。不要在 `Dispatch` 里同步等待自己的 `Term`。
- 持久化放在 `Term` 或提前发出的消息里。停机时还排在邮箱里、没来得及处理的消息会被丢掉，未完成的异步 `Call` 会尽量回 `ErrDead`。
