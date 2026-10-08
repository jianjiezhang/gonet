package proto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

const MaxPayload = 64 << 10

var (
	ErrTooLarge = errors.New("harbormgr: 超过最大长度")
	ErrEmpty    = errors.New("harbormgr: 空包")
)

// Frame 是 harbor 与 harbormgr 之间的一帧。
// 线上是 4 字节大端长度、2 字节大端命令号、JSON 正文。长度含命令号和 JSON。
// JSON 是该命令在 msg.go 里对应结构体，不再包一层 cmd。
type Frame struct {
	Cmd  uint16
	Data json.RawMessage
}

// Pack 把该命令的结构体编进 Data。
func Pack(cmd uint16, payload any) (Frame, error) {
	if cmd == 0 {
		return Frame{}, ErrEmpty
	}
	if payload == nil {
		return Frame{Cmd: cmd}, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Cmd: cmd, Data: b}, nil
}

// Decode 把 Data 解开到调用方给出的结构体。
func (f Frame) Decode(payload any) error {
	if payload == nil {
		return ErrEmpty
	}
	if len(f.Data) == 0 {
		return nil
	}
	return json.Unmarshal(f.Data, payload)
}

// Body 按命令号解开 Data。不认识的命令或 Data 无法解析时 ok 为假。
func (f Frame) Body() (any, bool) {
	var payload any
	switch f.Cmd {
	case CmdRegisterAddr:
		payload = &RegisterAddr{}
	case CmdRegisterName:
		payload = &RegisterName{}
	case CmdHeartbeat:
		payload = &Heartbeat{}
	case CmdQueryAddr:
		payload = &QueryAddr{}
	case CmdUnregisterName:
		payload = &UnregisterName{}
	case CmdUnregisterAddr:
		payload = &UnregisterAddr{}
	case CmdResult:
		payload = &Result{}
	default:
		return nil, false
	}
	if err := f.Decode(payload); err != nil {
		return nil, false
	}
	return payload, true
}

// Write 写出 4 字节大端长度、2 字节大端命令号和 JSON 正文。
func Write(w io.Writer, f Frame) error {
	if f.Cmd == 0 {
		return ErrEmpty
	}
	n := 2 + len(f.Data)
	if n > MaxPayload {
		return ErrTooLarge
	}
	buf := make([]byte, 4+n)
	binary.BigEndian.PutUint32(buf[:4], uint32(n))
	binary.BigEndian.PutUint16(buf[4:6], f.Cmd)
	copy(buf[6:], f.Data)
	return writeFull(w, buf)
}

// Read 读满一帧。长度不含自己，含命令号和 JSON。
func Read(r io.Reader) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < 2 {
		return Frame{}, ErrEmpty
	}
	if n > MaxPayload {
		return Frame{}, ErrTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Frame{}, err
	}
	cmd := binary.BigEndian.Uint16(body[:2])
	if cmd == 0 {
		return Frame{}, ErrEmpty
	}
	return Frame{Cmd: cmd, Data: append(json.RawMessage(nil), body[2:]...)}, nil
}

func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
