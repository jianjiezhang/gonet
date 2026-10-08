package gamenet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"game/lib/packet"

	"gonet"
)

var (
	ErrCodec      = errors.New("gamenet: 正文解码失败")
	ErrUnknownCmd = errors.New("gamenet: 未知命令")
)

// Codec 编解码 data。命令号由帧带着，不放进 data。
// 默认 data 是 payload 的 JSON。nil payload 的 data 为空，不写成 null。
type Codec interface {
	Encode(payload any) ([]byte, error)
	Decode(data []byte) (string, error)
}

type jsonCodec struct{}

func (jsonCodec) Encode(payload any) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func (jsonCodec) Decode(data []byte) (string, error) {
	return string(data), nil
}

var clientCodec Codec = jsonCodec{}

var (
	cmdMu    sync.RWMutex
	idByName = map[string]uint16{}
	nameByID = map[uint16]string{}
)

// RegisterCmd 登记命令名和线上的命令号。名字或命令号为 0 不行。
// 同一对重复登记无事。名字和命令号对不上已有登记时返回错误。
// 游戏协议在进程启动时登记，lib 自己不带具体命令。
func RegisterCmd(name string, id uint16) error {
	if name == "" || id == 0 {
		return ErrUnknownCmd
	}
	cmdMu.Lock()
	defer cmdMu.Unlock()
	if prev, ok := idByName[name]; ok && prev != id {
		return fmt.Errorf("%w: %s 已是 %d", ErrUnknownCmd, name, prev)
	}
	if prev, ok := nameByID[id]; ok && prev != name {
		return fmt.Errorf("%w: %d 已是 %s", ErrUnknownCmd, id, prev)
	}
	idByName[name] = id
	nameByID[id] = name
	return nil
}

func commandID(name string) (uint16, bool) {
	cmdMu.RLock()
	id, ok := idByName[name]
	cmdMu.RUnlock()
	return id, ok && id != 0
}

func commandName(id uint16) (string, bool) {
	cmdMu.RLock()
	name, ok := nameByID[id]
	cmdMu.RUnlock()
	return name, ok && name != ""
}

// SetCodec 替换 data 的编解码。nil 忽略。不要在已经有连接读写时换。
func SetCodec(c Codec) {
	if c == nil {
		return
	}
	clientCodec = c
}

// WriteMsg 写出一帧：4 字节长度、2 字节命令号、data。
func WriteMsg(w io.Writer, cmd string, payload any) error {
	id, ok := commandID(cmd)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownCmd, cmd)
	}
	b, err := clientCodec.Encode(payload)
	if err != nil {
		return err
	}
	return packet.Write(w, id, b)
}

// ReadMsg 读一帧并得到业务消息。已登记的命令名会 Unpack；未登记则返回信封。
// 帧错误原样返回；命令号或正文无法识别是 ErrCodec，连接不断。
func ReadMsg(r io.Reader) (gonet.MessageInterface, error) {
	id, data, err := packet.Read(r)
	if err != nil {
		return nil, err
	}
	name, ok := commandName(id)
	if !ok {
		return nil, fmt.Errorf("%w: cmd %d", ErrCodec, id)
	}
	text, err := clientCodec.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCodec, err)
	}
	out, err := gonet.Unpack(name, text)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, gonet.ErrUnknownCmd) {
		msg := &gonet.BaseMessage{Data: text}
		msg.SetCmd(name)
		return msg, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrCodec, err)
}

// FrameLog 是一条玩家协议日志：[send] 或 [recv]，后面是命令和 JSON 正文。
// 服务名由调用方所在的 actor 协程加上，这里不写服务名。
func FrameLog(dir, cmd string, payload any) string {
	if cmd == "" {
		cmd = "?"
	}
	line := "[" + dir + "] " + cmd
	if payload == nil {
		return line
	}
	b, err := json.Marshal(payload)
	if err != nil || string(b) == "null" || string(b) == "{}" {
		return line
	}
	if string(b) == `{"cmd":"`+cmd+`","data":""}` {
		return line
	}
	return line + " " + string(b)
}

// Cmd 读消息的命令名。
func Cmd(msg gonet.MessageInterface) string {
	if msg == nil {
		return ""
	}
	if c, ok := msg.(interface{ Command() string }); ok {
		return c.Command()
	}
	return ""
}

// Data 读信封上的 payload 字符串；已 Unpack 的业务消息通常为空。
func Data(msg gonet.MessageInterface) string {
	if msg == nil {
		return ""
	}
	if p, ok := msg.(interface{ Payload() string }); ok {
		return p.Payload()
	}
	return ""
}
