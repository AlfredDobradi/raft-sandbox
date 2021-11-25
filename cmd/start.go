package main

import (
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/axdx/raft-sandbox/internal/daemon"
)

type StartCmd struct {
	NodeID   string   `help:"The unique identifer of the node"`
	Hostname string   `help:"Hostname to bind service to"`
	Port     string   `help:"Port to listen on"`
	Nodes    []string `help:"Address of the nodes in the cluster"`
}

func (c *StartCmd) Run(ctx *Context) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	d := daemon.New()
	go d.Loop()

	<-sigs
	d.Stop()
	return nil
}
