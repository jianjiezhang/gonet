package packet

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	headerSize = 4
	cmdSize    = 2
	MaxPayload = 64 << 10
)

var (
	ErrTooLarge = errors.New("packet: 超过最大长度")
	ErrEmpty    = errors.New("packet: 空包")
)

// Write 写出一帧：4 字节大端长度、2 字节大端命令号、data。
// 长度含命令号和 data，不含长度自己。命令号为 0 视为空包。data 可以为空。
func Write(w io.Writer, cmd uint16, data []byte) error {
	if cmd == 0 {
		return ErrEmpty
	}
	n := cmdSize + len(data)
	if n > MaxPayload {
		return ErrTooLarge
	}
	buf := make([]byte, headerSize+n)
	binary.BigEndian.PutUint32(buf[:headerSize], uint32(n))
	binary.BigEndian.PutUint16(buf[headerSize:headerSize+cmdSize], cmd)
	copy(buf[headerSize+cmdSize:], data)
	return writeFull(w, buf)
}

// Read 读满一帧。长度不含自己，含命令号和 data。
func Read(r io.Reader) (uint16, []byte, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < cmdSize {
		return 0, nil, ErrEmpty
	}
	if n > MaxPayload {
		return 0, nil, ErrTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	cmd := binary.BigEndian.Uint16(body[:cmdSize])
	if cmd == 0 {
		return 0, nil, ErrEmpty
	}
	data := append([]byte(nil), body[cmdSize:]...)
	return cmd, data, nil
}

func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
