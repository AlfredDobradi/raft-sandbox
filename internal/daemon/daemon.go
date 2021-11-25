package daemon

import (
	"log"
	"math/rand"
	"time"
)

const (
	TermLengthMin int64 = 150
	TermLengthMax int64 = 500
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

type Daemon struct {
	stop  chan struct{}
	timer *time.Timer
	term  int
}

func New() *Daemon {
	return &Daemon{
		stop: make(chan struct{}),
	}
}

func (d *Daemon) Stop() {
	log.Println("Stopping daemon...")
	d.stop <- struct{}{}
}

func (d *Daemon) NewTerm() {
	duration := time.Duration(rnd.Int63n(TermLengthMax-TermLengthMin)+TermLengthMin) * time.Millisecond
	log.Printf("Starting term %d, duration: %s", d.term, duration)
	d.timer = time.NewTimer(duration)
	d.term += 1
}

func (d *Daemon) Loop() {
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
