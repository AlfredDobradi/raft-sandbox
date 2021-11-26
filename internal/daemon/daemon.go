package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
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
	HeartbeatTimer int64 = 300
	// TermLengthMin     int64 = 150
	// TermLengthMax     int64 = 500
	TermLengthMin int64 = 7000
	TermLengthMax int64 = 10000
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
var ErrVoteNotGranted error = fmt.Errorf("vote not granted")
var ErrAlreadyVoted error = fmt.Errorf("already voted")

type Node struct {
	id       string
	hostname string
	port     string
	nodes    []*url.URL
	listener *http.Server

	heartbeat       bool
	state           NodeState
	errors          chan error
	stop            chan struct{}
	heartbeatTicker *time.Ticker
	termTimer       *time.Timer
	term            int
	votedFor        string
}

func New(opts ...NodeOpt) (*Node, error) {
	node := &Node{
		stop:            make(chan struct{}),
		heartbeatTicker: time.NewTicker(time.Duration(HeartbeatTimer) * time.Millisecond),
	}

	for _, fn := range opts {
		if err := fn(node); err != nil {
			return nil, err
		}
	}

	return node, nil
}

func (n *Node) Stop() {
	logging.GetLogger().Println("Stopping daemon...")
	if n.listener != nil {
		if err := n.listener.Shutdown(context.Background()); err != nil {
			logging.GetLogger().Printf("ERROR: %v", err)
		} // todo add timeout
	}
	n.stop <- struct{}{}
}

func (n *Node) NewTerm() {
	termDuration := time.Duration(rnd.Int63n(TermLengthMax-TermLengthMin)+TermLengthMin) * time.Millisecond

	logging.GetLogger().Printf("Starting term %d, duration: %s", n.term, termDuration)
	n.termTimer = time.NewTimer(termDuration)
	if !n.heartbeat && n.votedFor == "" {
		n.term += 1

		n.Election()
	}
}

func (n *Node) Election() {
	n.setState(Candidate)
	n.votedFor = ""
	var votes int64 = 1

	wg := &sync.WaitGroup{}
	for _, node := range n.nodes {
		wg.Add(1)
		go func(node *url.URL) {
			defer wg.Done()
			nodeHost := fmt.Sprintf("%s:%s", node.Hostname(), node.Port())
			if err := n.RequestVote(nodeHost); err != nil {
				logging.GetLogger().Printf("ELECTION: [%s] Error getting vote: %v", nodeHost, err)
				return
			}

			logging.GetLogger().Printf("ELECTION: [%s] Received vote", nodeHost)
			atomic.AddInt64(&votes, 1)
		}(node)
	}
	wg.Wait()
	logging.GetLogger().Printf("Term: %d, Votes: %d", n.term, votes)

	if votes > int64(len(n.nodes)/2) {
		n.setState(Leader)
		n.Heartbeat()
	} else {
		n.setState(Follower)
	}
}

func (n *Node) RequestVote(host string) error {
	request := RequestVoteRequest{
		Term:        n.term,
		CandidateID: n.id,
	}
	logging.GetLogger().Printf("ELECTION: Asking %s for vote", host)

	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	body := bytes.NewReader(payload)
	r, err := http.Post(fmt.Sprintf("http://%s/RequestVote", host), "application/json", body)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}

	responseBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("body: %v", err)
	}

	var response RequestVoteResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}

	if !response.VoteGranted {
		return ErrVoteNotGranted
	}

	return nil
}

func (n *Node) Loop() {
	logging.GetLogger().Println("Starting daemon...")
	go n.Listen()
	n.NewTerm()
	running := true
	for running {
		select { // nolint
		case err := <-n.errors:
			logging.GetLogger().Printf("ERROR: %v", err)
		case <-n.termTimer.C:
			n.NewTerm()
		case <-n.heartbeatTicker.C:
			n.Heartbeat()
		case <-n.stop:
			running = false
		}
	}
}

func (n *Node) Heartbeat() {
	if n.state == Leader {
		wg := &sync.WaitGroup{}
		for _, node := range n.nodes {
			wg.Add(1)
			go func(node *url.URL) {
				defer wg.Done()

				nodeHost := fmt.Sprintf("%s:%s", node.Hostname(), node.Port())
				_, err := http.Post(fmt.Sprintf("http://%s/AppendEntry", nodeHost), "application/json", nil)
				if err != nil {
					logging.GetLogger().Printf("HEARTBEAT: [%s] Error sending heartbeat: %v", nodeHost, err)
				} else {
					logging.GetLogger().Printf("HEARTBEAT: [%s] Heartbeat sent", nodeHost)
				}
			}(node)
		}
		wg.Wait()
	}
}

func (n *Node) Vote(nodeID string) error {
	if n.votedFor != "" || n.state != Follower {
		return ErrAlreadyVoted
	}

	n.votedFor = nodeID
	return nil
}

func (n *Node) HandleHeartbeat() {
	logging.GetLogger().Printf("HEARTBEAT: Received")
	n.heartbeat = true
	n.setState(Follower)
}

func (n *Node) setState(state NodeState) {
	if n.state != state {
		logging.GetLogger().Printf("New state: %s", state)
		n.state = state
	}
}
