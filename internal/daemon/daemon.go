package daemon

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

type NodeState string

const (
	Follower  NodeState = "FOLLOWER"
	Candidate NodeState = "CANDIDATE"
	Leader    NodeState = "LEADER"
)

const (
	ElectionLengthMin int64 = 150
	ElectionLengthMax int64 = 300
	// TermLengthMin     int64 = 150
	// TermLengthMax     int64 = 500
	TermLengthMin int64 = 3000
	TermLengthMax int64 = 6000
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

type Node struct {
	hostname string
	port     string
	nodes    []*url.URL
	listener *http.Server

	state  NodeState
	errors chan error
	stop   chan struct{}
	timer  *time.Timer
	term   int
	voted  bool
	votes  int
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

func (n *Node) Stop() {
	log.Println("Stopping daemon...")
	if n.listener != nil {
		if err := n.listener.Shutdown(context.Background()); err != nil {
			log.Printf("ERROR: %v", err)
		} // todo add timeout
	}
	n.stop <- struct{}{}
}

func (n *Node) NewTerm() {
	termDuration := time.Duration(rnd.Int63n(TermLengthMax-TermLengthMin)+TermLengthMin) * time.Millisecond

	logging.GetLogger().Log(fmt.Sprintf("Starting term %d, duration: %s", n.term, termDuration))
	n.timer = time.NewTimer(termDuration)
	n.voted = false
	n.term += 1

	electionDuration := time.Duration(rnd.Int63n(ElectionLengthMax-ElectionLengthMin)+ElectionLengthMin) * time.Millisecond
	// logging.GetLogger().Log(fmt.Sprintf("Starting election, timeout: %s", electionDuration))

	ctx, cancel := context.WithTimeout(context.Background(), electionDuration)
	defer cancel()
	log.Printf("ELECTION: Starting election with %s timeout", electionDuration)
	n.Election(ctx)
}

func (n *Node) Election(ctx context.Context) {
	n.votes = 0
	n.state = Candidate
	wg := &sync.WaitGroup{}
	for _, node := range n.nodes {
		wg.Add(1)
		go func(node *url.URL) {
			defer wg.Done()
			nodeHost := fmt.Sprintf("%s:%s", node.Hostname(), node.Port())
			log.Printf("ELECTION: Asking %s for vote", nodeHost)
			r, err := http.Get(fmt.Sprintf("http://%s/RequestVote", nodeHost))
			if err != nil {
				log.Printf("ELECTION: [%s] Error sending vote request: %v", nodeHost, err)
				return
			}
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				if r.StatusCode == http.StatusOK {
					n.votes += 1
					log.Printf("ELECTION: [%s] We received the vote, but can't read body: %v", nodeHost, err)
				} else {
					log.Printf("ELECTION: [%s] Getting vote failed, but can't read body: %v", nodeHost, err)
				}
				return
			}
			if r.StatusCode != http.StatusOK {
				log.Printf("ELECTION: [%s] Error receiving vote: %s", nodeHost, body)
				return
			}

			log.Printf("ELECTION: [%s] Got vote: %s", nodeHost, body)
			n.votes += 1
		}(node)
	}
	wg.Wait()
	log.Printf("Term: %d, Votes: %d", n.term, n.votes)
}

func (n *Node) Loop() {
	log.Println("Starting daemon...")
	go n.Listen()
	n.NewTerm()
	running := true
	for running {
		select { // nolint
		case err := <-n.errors:
			log.Printf("ERROR: %v", err)
		case <-n.timer.C:
			n.NewTerm()
		case <-n.stop:
			running = false
		}
	}
}

func (n *Node) Vote() error {
	if n.voted {
		return fmt.Errorf("already voted")
	}

	n.voted = true
	return nil
}
