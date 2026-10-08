package gonet_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"gonet"
)

type pingMsg struct{ gonet.BaseMessage }
type getNMsg struct{ gonet.BaseMessage }
type getCountMsg struct{ gonet.BaseMessage }
type boomMsg struct{ gonet.BaseMessage }
type askFromMsg struct{ gonet.BaseMessage }

type scheduleMsg struct {
	gonet.BaseMessage
	d time.Duration
}

type toPeerMsg struct {
	gonet.BaseMessage
	to uint64
}

type sendPeerMsg struct {
	gonet.BaseMessage
	to   uint64
	name string
}

type toPeerNameMsg struct {
	gonet.BaseMessage
	name string
}

type registerMsg struct {
	gonet.BaseMessage
	name string
}

type unregisterMsg struct{ gonet.BaseMessage }

type exitMsg struct{ gonet.BaseMessage }

type suspendSelfMsg struct{ gonet.BaseMessage }

type holdMsg struct {
	gonet.BaseMessage
	started chan struct{}
	release chan struct{}
}

type fromSink struct {
	gonet.ActorContext
	got chan uint64
}

func (a *fromSink) Dispatch(e gonet.Envelope) {
	a.got <- e.From
}

type echoActor struct {
	gonet.ActorContext
	n         int
	term      atomic.Bool
	peerReply *gonet.Envelope
}

func (a *echoActor) Term() { a.term.Store(true) }

func (a *echoActor) Dispatch(e gonet.Envelope) {
	switch m := e.Msg.(type) {
	case pingMsg:
		a.n++
		e.Reply("pong")
	case getCountMsg:
		e.Reply(a.n)
	case getNMsg:
		e.Reply(a.Self())
	case boomMsg:
		panic("boom")
	case holdMsg:
		close(m.started)
		<-m.release
	case askFromMsg:
		e.Reply(e.From)
	case sendPeerMsg:
		var err error
		if m.name != "" {
			err = a.SendMemoryName(m.name, askFromMsg{})
		} else {
			err = a.SendMemory(m.to, askFromMsg{})
		}
		e.Reply(err)
	case toPeerMsg:
		env := e
		a.peerReply = &env
		if _, err := e.CallMemory(m.to, time.Second, askFromMsg{}); err != nil {
			a.peerReply = nil
			e.Reply(err.Error())
		}
	case toPeerNameMsg:
		env := e
		a.peerReply = &env
		if _, err := e.CallMemoryName(m.name, time.Second, askFromMsg{}); err != nil {
			a.peerReply = nil
			e.Reply(err)
		}
	case *gonet.CallResponse:
		if a.peerReply == nil {
			return
		}
		er := *a.peerReply
		a.peerReply = nil
		if m.Err != nil {
			er.Reply(m.Err)
			return
		}
		er.Reply(m.Value)
	case registerMsg:
		e.Reply(a.Register(m.name))
	case unregisterMsg:
		e.Reply(a.Unregister())
	case scheduleMsg:
		msg := pingMsg{}
		msg.SetCmd("ping")
		_, err := e.Timeout(m.d, msg)
		e.Reply(err)
	case *gonet.BaseMessage:
		if m.Command() == "ping" {
			a.n++
			e.Reply("pong")
		}
	case exitMsg:
		e.Exit()
		e.Reply("bye")
	case suspendSelfMsg:
		_, err := gonet.SuspendCallMemory(context.Background(), a.Self(), pingMsg{})
		e.Reply(err)
	}
}

type initRegActor struct {
	gonet.ActorContext
	name string
	reg  error
}

func (a *initRegActor) Init() error {
	a.reg = a.Register(a.name)
	return a.reg
}

type failAfterRegActor struct {
	gonet.ActorContext
	name string
}

func (a *failAfterRegActor) Init() error {
	if err := a.Register(a.name); err != nil {
		return err
	}
	return errors.New("init failed")
}

type failInitActor struct {
	gonet.ActorContext
}

func (a *failInitActor) Init() error { return errors.New("init failed") }

type typeA struct{ gonet.BaseMessage }
type typeB struct{ gonet.BaseMessage }

type swapHost struct {
	gonet.ActorContext
}

func (a *swapHost) Init() error {
	h := func(e gonet.Envelope) { e.Reply(fmt.Sprintf("%T", e.Msg)) }
	if err := gonet.RegisterCmd(a, "swap", func() gonet.MessageInterface { return &typeA{} }, h); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, "swap", func() gonet.MessageInterface { return &typeB{} }, h)
}

type asyncCaller struct {
	gonet.ActorContext
	got chan *gonet.CallResponse
}

func (a *asyncCaller) Init() error {
	a.got = make(chan *gonet.CallResponse, 1)
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, func(e gonet.Envelope) {
		m, ok := e.Msg.(*gonet.CallResponse)
		if !ok {
			return
		}
		a.got <- m
	})
}

type slowInitActor struct {
	gonet.ActorContext
	d time.Duration
}

func (a *slowInitActor) Init() error {
	time.Sleep(a.d)
	return nil
}

type slowTermActor struct {
	gonet.ActorContext
	d time.Duration
}

func (a *slowTermActor) Term() {
	time.Sleep(a.d)
}

type bareHost struct {
	gonet.ActorContext
}

func (a *bareHost) Init() error {
	return gonet.RegisterCmd(a, "ping", func() gonet.MessageInterface { return &pingMsg{} }, func(e gonet.Envelope) {
		e.Reply("pong")
	})
}
