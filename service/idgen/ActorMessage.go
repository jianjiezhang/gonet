package idgen

import "gonet"

const (
	CmdAlloc = "idgen.alloc"
	CmdQuery = "idgen.query"
	CmdList  = "idgen.list"

	cmdSaved = "idgen.saved"
)

// AllocMsg 为这一种类取下一个号。Kind 由调用方定，例如 role、guild、room。
type AllocMsg struct {
	gonet.BaseMessage
	Kind string `json:"kind"`
}

// QueryMsg 查询这一种类已经发出的最大号。不分配。
type QueryMsg struct {
	gonet.BaseMessage
	Kind string `json:"kind"`
}

// ListMsg 列出当前管着的全部种类和各自的最大号。
type ListMsg struct {
	gonet.BaseMessage
}

// Reply 是分配或查询的回复。ID 是十进制字符串，还没发过是 "0"。
type Reply struct {
	Err  string `json:"err,omitempty"`
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}

// Item 是一种编号的当前水位。
type Item struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ListReply 是全部种类。
type ListReply struct {
	Err   string `json:"err,omitempty"`
	Items []Item `json:"items,omitempty"`
}

// savedMsg 是水位已经写入 Seq 之后的回执。只在本进程传递。
type savedMsg struct {
	gonet.BaseMessage
	N uint64 `json:"-"`
}
