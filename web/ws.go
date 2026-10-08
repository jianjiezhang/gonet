package web

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const (
	wsGUID       = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	opText  byte = 0x1
	opBin   byte = 0x2
	opClose byte = 0x8
	opPing  byte = 0x9
	opPong  byte = 0xA

	// 一帧游戏包最大 4 字节长度 + 64KiB。
	maxWS = 4 + 64<<10
)

var errUpgrade = errors.New("web: 需要 websocket")

// wsConn 是升级完成后的一条 WebSocket。读的是浏览器帧（必须带掩码），写的是服务端帧（不带掩码）。
type wsConn struct {
	conn net.Conn
	r    *bufio.Reader
	wmu  sync.Mutex
}

func upgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !websocketUpgrade(r) {
		http.Error(w, "需要 websocket", http.StatusBadRequest)
		return nil, errUpgrade
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "无法升级", http.StatusInternalServerError)
		return nil, errors.New("web: 无法升级")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	accept := acceptKey(r.Header.Get("Sec-WebSocket-Key"))
	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, r: rw.Reader}, nil
}

func websocketUpgrade(r *http.Request) bool {
	if r.Method != http.MethodGet || r.Header.Get("Sec-WebSocket-Version") != "13" || r.Header.Get("Sec-WebSocket-Key") == "" {
		return false
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || !headerToken(r.Header, "Connection", "upgrade") {
		return false
	}
	return true
}

func headerToken(h http.Header, key, token string) bool {
	for _, part := range strings.Split(h.Get(key), ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Read 读浏览器发来的一整帧。分片和未掩码帧直接当错误，让会话关掉。
func (c *wsConn) Read() (byte, []byte, error) {
	return readWS(c.r, true)
}

func (c *wsConn) WriteBinary(payload []byte) error {
	return c.write(opBin, payload)
}

func (c *wsConn) WritePong(payload []byte) error {
	return c.write(opPong, payload)
}

func (c *wsConn) Close() error {
	_ = c.write(opClose, nil)
	return c.conn.Close()
}

func (c *wsConn) write(opcode byte, payload []byte) error {
	if len(payload) > maxWS {
		return errors.New("web: 帧过大")
	}
	hdr := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n <= 65535:
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(n))
		hdr = append(hdr, 126)
		hdr = append(hdr, b[:]...)
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, 127)
		hdr = append(hdr, b[:]...)
	}
	buf := append(hdr, payload...)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.conn.Write(buf)
	return err
}

// readWS 读一帧。fromClient 为真时要求掩码，服务端帧则不能带掩码。
func readWS(r *bufio.Reader, fromClient bool) (byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	if hdr[0]&0x70 != 0 || hdr[0]&0x80 == 0 || hdr[0]&0x0f == 0 {
		return 0, nil, errors.New("web: 不支持分片")
	}
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	if fromClient != masked {
		return 0, nil, errors.New("web: 掩码不对")
	}
	n := uint64(hdr[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > maxWS {
		return 0, nil, errors.New("web: 帧过大")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	switch opcode {
	case opText, opBin, opClose, opPing, opPong:
		return opcode, payload, nil
	default:
		return 0, nil, errors.New("web: 未知帧")
	}
}
