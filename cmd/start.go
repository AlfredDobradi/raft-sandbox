package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/axdx/raft-sandbox/internal/config"
	"gitlab.com/axdx/raft-sandbox/internal/daemon"
)

type StartCmd struct {
	NodeID   string   `help:"The unique identifier of the node"`
	Hostname string   `help:"Hostname to bind service to" default:"0.0.0.0"`
	Port     string   `help:"Port to listen on" default:"31337"`
	Nodes    []string `help:"Address of the nodes in the cluster"`
}

func (c *StartCmd) Run(ctx *Context) error {
	log.Printf("Debug mode: %t", ctx.Debug)
	config.SetDebug(ctx.Debug)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	d, err := daemon.New(
		daemon.WithHostname(c.Hostname),
		daemon.WithPort(c.Port),
		daemon.WithNodeList(c.Nodes),
		daemon.WithNodeID(c.NodeID),
	)
	if err != nil {
		log.Panicf("error creating daemon: %v", err)
	}

	go d.Loop()

	<-sigs
	d.Stop()
	return nil
}
