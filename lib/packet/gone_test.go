package packet

import (
	"io"
	"net"
	"testing"
)

func TestIsGone(t *testing.T) {
	if !IsGone(io.EOF) || !IsGone(net.ErrClosed) {
		t.Fatal("eof/closed")
	}
	if IsGone(ErrEmpty) || IsGone(nil) {
		t.Fatal("not gone")
	}
}
