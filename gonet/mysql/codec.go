package mysql

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
)

var ErrNilCodec = errors.New("gonet/mysql: codec 不能为 nil")

// Codec 把内存对象编成库里的 []byte，读回来再解开。gonet 不解释内容。
type Codec interface {
	Encode(v any) ([]byte, error)
	Decode(data []byte, v any) error
}

type jsonCodec struct{}

func JSON() Codec { return jsonCodec{} }

func (jsonCodec) Encode(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

func (jsonCodec) Decode(data []byte, v any) error {
	if v == nil {
		return ErrNilCodec
	}
	if len(data) == 0 {
		return io.ErrUnexpectedEOF
	}
	return json.Unmarshal(data, v)
}

type gzipJSONCodec struct{}

func GzipJSON() Codec { return gzipJSONCodec{} }

func (gzipJSONCodec) Encode(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (gzipJSONCodec) Decode(data []byte, v any) error {
	if v == nil {
		return ErrNilCodec
	}
	if len(data) == 0 {
		return io.ErrUnexpectedEOF
	}
	raw := data
	if isGzip(data) {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		raw, err = io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(raw, v)
}

func isGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}
