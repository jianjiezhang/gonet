package mysql

import (
	"bytes"
	"testing"
)

type doc struct {
	Ver  int      `json:"ver"`
	List []string `json:"list"`
}

func TestJSONCodec(t *testing.T) {
	c := JSON()
	in := doc{Ver: 1, List: []string{"a"}}
	b, err := c.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	var out doc
	if err := c.Decode(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Ver != 1 || len(out.List) != 1 || out.List[0] != "a" {
		t.Fatalf("got %+v", out)
	}
}

func TestGzipJSONCodec(t *testing.T) {
	c := GzipJSON()
	in := doc{Ver: 2, List: []string{"x", "y"}}
	b, err := c.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	if !isGzip(b) {
		t.Fatal("expected gzip")
	}
	var out doc
	if err := c.Decode(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Ver != 2 || len(out.List) != 2 {
		t.Fatalf("got %+v", out)
	}
}

func TestGzipJSONAcceptsPlainJSON(t *testing.T) {
	raw, err := JSON().Encode(doc{Ver: 3})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) {
		t.Fatal("plain json should not be gzip")
	}
	var out doc
	if err := GzipJSON().Decode(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Ver != 3 {
		t.Fatalf("got %+v", out)
	}
}

func TestCodecEmpty(t *testing.T) {
	if err := JSON().Decode(nil, &doc{}); err == nil {
		t.Fatal("expected error")
	}
	if err := GzipJSON().Decode(nil, &doc{}); err == nil {
		t.Fatal("expected error")
	}
}
