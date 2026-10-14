package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9999", "debug address")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		slog.Error("dial", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	slog.Info("connected", "addr", *addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		_, _ = io.Copy(crlfWriter{os.Stdout}, conn)
		stop()
	}()

	ed := newLineEditor(os.Stdin, os.Stdout)
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := ed.read(ctx)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintln(conn, line); err != nil {
			slog.Error("write", "err", err)
			return
		}
	}
}

// crlfWriter 在 raw 终端里把单独的 \n 写成 \r\n，否则下一行会从上一行末尾接着打。
type crlfWriter struct{ w io.Writer }

func (c crlfWriter) Write(p []byte) (int, error) {
	var out []byte
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, b)
	}
	if _, err := c.w.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}
