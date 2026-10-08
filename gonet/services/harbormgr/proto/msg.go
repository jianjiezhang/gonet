// Package proto 是 harbor 与 harbormgr 的 TCP 协议。
// 每一帧是 4 字节大端长度、2 字节大端命令号、JSON 正文。
//
// 请求（harbor -> harbormgr）：
//
// 节点 ID 由 harbormgr 在 registeraddr 时分配，不来自配置。
// NodeID 为 0 表示申请新 ID。非 0 时认回该编号：记录还在就续上；记录没有了（过期或 harbormgr 重启）且编号没被占用，就按原编号重建，并把分配计数抬到这个编号。
// 编号已被其他连接占用时返回 taken。地址只是这条记录当前的联系地址，可以被拥有者改掉。
//
//	Cmd  Data
//	1    RegisterAddr
//	2    RegisterName
//	3    Heartbeat
//	4    QueryAddr
//	5    UnregisterName
//	6    UnregisterAddr
//
// 回复帧的命令号固定是 7。JSON 仍是命令号 + 正文。
// 这里的命令号与请求相同，用来决定正文解开成哪个结果结构：
//
//	Cmd  Data
//	1    RegisterAddrResult
//	2    RegisterNameResult
//	3    HeartbeatResult
//	4    QueryAddrResult
//	5    UnregisterNameResult
//	6    UnregisterAddrResult
package proto

import "encoding/json"

const (
	CmdRegisterAddr   uint16 = 1
	CmdRegisterName   uint16 = 2
	CmdHeartbeat      uint16 = 3
	CmdQueryAddr      uint16 = 4
	CmdUnregisterName uint16 = 5
	CmdUnregisterAddr uint16 = 6
	CmdResult         uint16 = 7
)

const (
	ErrInvalid = "invalid"
	ErrUnknown = "unknown"
	ErrTaken   = "taken"
)

// RegisterAddr 是 harbor 登记本节点地址时的 Data。
// NodeID 为 0 时由 harbormgr 分配新 ID。非 0 时认回该编号；目录里没有这条记录时按原编号重建。
type RegisterAddr struct {
	ID     uint64 `json:"id,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

// RegisterName 是 harbor 把别名挂到该节点时的 Data。
type RegisterName struct {
	ID     uint64 `json:"id,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Heartbeat 是 harbor 刷新节点存活时间时的 Data。
type Heartbeat struct {
	ID     uint64 `json:"id,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

// QueryAddr 是 harbor 按别名查地址时的 Data。
type QueryAddr struct {
	ID   uint64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// UnregisterName 是 harbor 从本节点摘掉别名时的 Data。
type UnregisterName struct {
	ID     uint64 `json:"id,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Name   string `json:"name,omitempty"`
}

// UnregisterAddr 是 harbor 摘掉本节点及其全部别名时的 Data。
type UnregisterAddr struct {
	ID     uint64 `json:"id,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

// Result 是回复帧的 JSON，本身也是命令号 + 正文。
// Cmd 与对应请求相同，Data 是该命令的结果结构。
type Result struct {
	Cmd  uint16          `json:"cmd"`
	Data json.RawMessage `json:"data,omitempty"`
}

// NewResult 把该命令的结果结构编进 Data。
func NewResult(cmd uint16, payload any) (Result, error) {
	if cmd == 0 {
		return Result{}, ErrEmpty
	}
	if payload == nil {
		return Result{Cmd: cmd}, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	return Result{Cmd: cmd, Data: b}, nil
}

// Decode 把 Data 解开到调用方给出的结果结构。
func (r Result) Decode(payload any) error {
	if payload == nil {
		return ErrEmpty
	}
	if len(r.Data) == 0 {
		return nil
	}
	return json.Unmarshal(r.Data, payload)
}

// Body 按 Cmd 解开 Data。不认识的命令或 Data 无法解析时 ok 为假。
func (r Result) Body() (any, bool) {
	var payload any
	switch r.Cmd {
	case CmdRegisterAddr:
		payload = &RegisterAddrResult{}
	case CmdRegisterName:
		payload = &RegisterNameResult{}
	case CmdHeartbeat:
		payload = &HeartbeatResult{}
	case CmdQueryAddr:
		payload = &QueryAddrResult{}
	case CmdUnregisterName:
		payload = &UnregisterNameResult{}
	case CmdUnregisterAddr:
		payload = &UnregisterAddrResult{}
	default:
		return nil, false
	}
	if err := r.Decode(payload); err != nil {
		return nil, false
	}
	return payload, true
}

// IDOf 取出结果里的请求编号，用来和发出的请求配对。
func IDOf(body any) (uint64, bool) {
	switch m := body.(type) {
	case *RegisterAddrResult:
		return m.ID, true
	case *RegisterNameResult:
		return m.ID, true
	case *HeartbeatResult:
		return m.ID, true
	case *QueryAddrResult:
		return m.ID, true
	case *UnregisterNameResult:
		return m.ID, true
	case *UnregisterAddrResult:
		return m.ID, true
	default:
		return 0, false
	}
}

// Status 是每条命令结果都有的编号、成败和错误。命令自己的字段写在各自的结果结构里。
type Status struct {
	ID  uint64 `json:"id,omitempty"`
	OK  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
}

// RegisterAddrResult 是登记地址的回复 Data。
// Fresh 为真表示这是新记录，旧别名没有保留。
type RegisterAddrResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Fresh  bool   `json:"fresh,omitempty"`
}

// RegisterNameResult 是登记别名的回复 Data。别名已被占用时 Addr 是当前登记地址。
type RegisterNameResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Name   string `json:"name,omitempty"`
}

// HeartbeatResult 是心跳的回复 Data。
type HeartbeatResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

// QueryAddrResult 是按别名查地址的回复 Data。
type QueryAddrResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Name   string `json:"name,omitempty"`
}

// UnregisterNameResult 是摘掉别名的回复 Data。
type UnregisterNameResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Name   string `json:"name,omitempty"`
}

// UnregisterAddrResult 是摘掉节点的回复 Data。
type UnregisterAddrResult struct {
	Status
	NodeID uint64 `json:"node,omitempty"`
	Addr   string `json:"addr,omitempty"`
}
