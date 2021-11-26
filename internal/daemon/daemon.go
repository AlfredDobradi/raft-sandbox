package daemon

import (
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"time"

	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

const (
	ElectionLengthMin int64 = 150
	ElectionLengthMax int64 = 300
	TermLengthMin     int64 = 150
	TermLengthMax     int64 = 500
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

type Node struct {
	hostname string
	port     string
	nodes    []*url.URL

	stop  chan struct{}
	timer *time.Timer
	term  int
}

func New(opts ...NodeOpt) (*Node, error) {
	node := &Node{
		stop: make(chan struct{}),
	}

	for _, fn := range opts {
		if err := fn(node); err != nil {
			return nil, err
		}
	}

	return node, nil
}

func (d *Node) Stop() {
	log.Println("Stopping daemon...")
	d.stop <- struct{}{}
}

func (d *Node) NewTerm() {
	// electionDuration := time.Duration(rnd.Int63n(ElectionLengthMax-ElectionLengthMin)+ElectionLengthMin) * time.Millisecond
	// logging.GetLogger().Log(fmt.Sprintf("Starting election, timeout: %s", electionDuration))

	// ctx, cancel := context.WithTimeout(context.Background(), electionDuration)
	// d.Election(ctx)

	termDuration := time.Duration(rnd.Int63n(TermLengthMax-TermLengthMin)+TermLengthMin) * time.Millisecond

	logging.GetLogger().Log(fmt.Sprintf("Starting term %d, duration: %s", d.term, termDuration))
	d.timer = time.NewTimer(termDuration)
	d.term += 1
}

// func (d *Daemon) Election(ctx context.Context) {

// }

func (d *Node) Loop() {
	log.Println("Starting daemon...")
	d.NewTerm()
	running := true
	for running {
		select { // nolint
		case <-d.timer.C:
			d.NewTerm()
		case <-d.stop:
			running = false
		}
	}
}
