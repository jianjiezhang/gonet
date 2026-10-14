package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type lineEditor struct {
	in      io.Reader
	out     io.Writer
	history []string
	histIdx int
}

func newLineEditor(in io.Reader, out io.Writer) *lineEditor {
	return &lineEditor{in: in, out: out, histIdx: -1}
}

func (e *lineEditor) read(ctx context.Context) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return e.readPlain(ctx)
	}
	old, err := term.GetState(fd)
	if err != nil {
		return e.readPlain(ctx)
	}
	if _, err := term.MakeRaw(fd); err != nil {
		return e.readPlain(ctx)
	}
	defer term.Restore(fd, old)

	var buf []byte
	cursor := 0
	e.histIdx = len(e.history)
	draft := ""
	b := make([]byte, 8)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := e.in.Read(b[:1])
		if n == 0 && err != nil {
			return "", err
		}
		c := b[0]
		switch {
		case c == 3: // Ctrl-C
			return "", io.EOF
		case c == 4: // Ctrl-D
			if len(buf) == 0 {
				return "", io.EOF
			}
		case c == '\r' || c == '\n':
			_, _ = fmt.Fprint(e.out, "\r\n")
			line := string(buf)
			e.pushHistory(line)
			return line, nil
		case c == 127 || c == 8: // backspace
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
				e.redraw(buf, cursor)
			}
		case c == 27:
			seq := e.readEscape(b)
			switch seq {
			case "[A":
				buf, cursor = e.histUp(buf, &draft)
				e.redraw(buf, cursor)
			case "[B":
				buf, cursor = e.histDown(buf, &draft)
				e.redraw(buf, cursor)
			case "[C":
				if cursor < len(buf) {
					cursor++
					e.redraw(buf, cursor)
				}
			case "[D":
				if cursor > 0 {
					cursor--
					e.redraw(buf, cursor)
				}
			}
		case c >= 32 && c < 127:
			buf = append(buf[:cursor], append([]byte{c}, buf[cursor:]...)...)
			cursor++
			e.redraw(buf, cursor)
		}
	}
}

func (e *lineEditor) readEscape(b []byte) string {
	n, err := e.in.Read(b[:1])
	if n == 0 || err != nil || b[0] != '[' {
		return ""
	}
	n, err = e.in.Read(b[:1])
	if n == 0 || err != nil {
		return ""
	}
	return "[" + string(b[0])
}

func (e *lineEditor) histUp(buf []byte, draft *string) ([]byte, int) {
	if len(e.history) == 0 {
		return buf, len(buf)
	}
	if e.histIdx == len(e.history) {
		*draft = string(buf)
	}
	if e.histIdx > 0 {
		e.histIdx--
	}
	s := e.history[e.histIdx]
	return []byte(s), len(s)
}

func (e *lineEditor) histDown(buf []byte, draft *string) ([]byte, int) {
	if e.histIdx < 0 {
		return buf, len(buf)
	}
	if e.histIdx < len(e.history) {
		e.histIdx++
	}
	if e.histIdx == len(e.history) {
		s := *draft
		return []byte(s), len(s)
	}
	s := e.history[e.histIdx]
	return []byte(s), len(s)
}

func (e *lineEditor) pushHistory(line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	if n := len(e.history); n > 0 && e.history[n-1] == line {
		return
	}
	e.history = append(e.history, line)
}

func (e *lineEditor) redraw(buf []byte, cursor int) {
	_, _ = fmt.Fprint(e.out, "\r\x1b[K> "+string(buf))
	if back := len(buf) - cursor; back > 0 {
		_, _ = fmt.Fprintf(e.out, "\x1b[%dD", back)
	}
}

func (e *lineEditor) readPlain(ctx context.Context) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		var b []byte
		tmp := make([]byte, 1)
		for {
			n, err := e.in.Read(tmp)
			if n > 0 {
				if tmp[0] == '\n' {
					ch <- res{s: strings.TrimRight(string(b), "\r")}
					return
				}
				b = append(b, tmp[0])
			}
			if err != nil {
				ch <- res{err: err}
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		if r.err == nil {
			e.pushHistory(r.s)
		}
		return r.s, r.err
	}
}
