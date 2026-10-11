// idgenmsg.go 是发号服务的消息。
package actormsg

import "gonet"

const (
	CmdIdgenAlloc     = "idgen.alloc"
	CmdIdgenQuery     = "idgen.query"
	CmdIdgenList      = "idgen.list"
	CmdIdgenReply     = "idgen.reply"
	CmdIdgenListReply = "idgen.list.reply"
)

// IdgenAllocMsg 为这一种类取下一个号。Kind 由调用方定，例如 role、guild、room。
type IdgenAllocMsg struct {
	gonet.BaseMessage
	Kind string `json:"kind"`
}

// IdgenQueryMsg 查询这一种类已经发出的最大号。不分配。
type IdgenQueryMsg struct {
	gonet.BaseMessage
	Kind string `json:"kind"`
}

// IdgenListMsg 列出当前管着的全部种类和各自的最大号。
type IdgenListMsg struct {
	gonet.BaseMessage
}

// IdgenReply 是分配或查询的回复。ID 是十进制字符串，还没发过是 "0"。
type IdgenReply struct {
	gonet.BaseMessage
	Err  string `json:"err,omitempty"`
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}

// IdgenItem 是一种编号的当前水位。
type IdgenItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// IdgenListReply 是全部种类。
type IdgenListReply struct {
	gonet.BaseMessage
	Err   string      `json:"err,omitempty"`
	Items []IdgenItem `json:"items,omitempty"`
}
