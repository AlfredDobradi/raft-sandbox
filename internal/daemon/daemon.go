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

	"github.com/google/uuid"
	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

type NodeState string

const (
	Follower  NodeState = "FOLLOWER"
	Candidate NodeState = "CANDIDATE"
	Leader    NodeState = "LEADER"
)

const (
	HeartbeatTimer int64 = 1000
	// TermLengthMin     int64 = 150
	// TermLengthMax     int64 = 500
	TermLengthMin      int64 = 7000
	TermLengthMax      int64 = 10000
	RequestVoteRetries int   = 3
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

// TODO Create error types
var ErrVoteNotGranted error = fmt.Errorf("vote not granted")
var ErrTermOutdated error = fmt.Errorf("term is out of date")
var ErrLeaderCantVote error = fmt.Errorf("leader requested to vote")
var ErrAlreadyVoted error = fmt.Errorf("already voted")
var ErrElectionTimeout error = fmt.Errorf("election timed out")

type Node struct {
	id       string
	hostname string
	port     string
	registry []Connection
	listener *http.Server

	heartbeat       bool
	state           NodeState
	errors          chan error
	stop            chan struct{}
	heartbeatTicker *time.Ticker
	termTimer       *time.Timer
	currentTerm     int
	votedFor        string
}

type Connection struct {
	url      *url.URL
	lastSeen *time.Time
}

func New(opts ...NodeOpt) (*Node, error) {
	node := &Node{
		stop:            make(chan struct{}),
		heartbeatTicker: time.NewTicker(time.Duration(HeartbeatTimer) * time.Millisecond),
		registry:        make([]Connection, 0),
	}

	for _, fn := range opts {
		if err := fn(node); err != nil {
			return nil, err
		}
	}

	if node.id == "" {
		node.id = uuid.New().String()
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

	logging.GetLogger().Printf("Starting term %d, duration: %s", n.currentTerm, termDuration)
	n.termTimer = time.NewTimer(termDuration)
	if n.state == Follower && !n.heartbeat && n.votedFor == "" {
		n.currentTerm += 1

		ctx, cancel := context.WithTimeout(context.Background(), termDuration)
		defer cancel()
		if err := n.Election(ctx, termDuration); err != nil {
			n.errors <- err
		}
	}
}

func (n *Node) Election(ctx context.Context, timeout time.Duration) error {
	n.setState(Candidate)
	n.votedFor = n.id
	var votes int64 = 1

	select {
	case <-ctx.Done():
		return ErrElectionTimeout
	default:
		wg := &sync.WaitGroup{}

		fn := func(node *Connection) {
			defer wg.Done()
			nodeHost := fmt.Sprintf("%s:%s", node.url.Hostname(), node.url.Port())
			if err := n.RequestVote(ctx, timeout, nodeHost); err != nil {
				e := fmt.Errorf("ELECTION: [%s] Error getting vote: %w", nodeHost, err)
				n.errors <- e
				logging.GetLogger().Println(e.Error())
				return
			}

			logging.GetLogger().Printf("ELECTION: [%s] Received vote", nodeHost)
			atomic.AddInt64(&votes, 1)
		}

		for i := range n.registry {
			wg.Add(1)
			go fn(&n.registry[i])
		}
		wg.Wait()
		logging.GetLogger().Printf("ELECTION: Term: %d, Votes: %d", n.currentTerm, votes)

		if votes > int64(len(n.registry)/2) {
			n.setState(Leader)
			n.Heartbeat()
		} else {
			n.setState(Follower)
		}
		return nil
	}
}

func (n *Node) RequestVote(ctx context.Context, timeout time.Duration, host string) error {
	request := RequestVoteRequest{
		Term:        n.currentTerm,
		CandidateID: n.id,
	}
	logging.GetLogger().Printf("ELECTION: Asking %s for vote", host)

	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	body := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/RequestVote", host), body)
	if err != nil {
		return fmt.Errorf("couldn't create request: %w", err)
	}

	client := http.Client{
		Timeout: timeout,
	}

	var r *http.Response
	var httpErr error
	for i := 0; i < RequestVoteRetries; i++ {
		r, httpErr = client.Do(req)
		if httpErr == nil {
			break
		}
	}
	if httpErr != nil {
		return fmt.Errorf("post: %w", httpErr)
	} else if r.StatusCode != http.StatusOK {
		return fmt.Errorf("post: %s", r.Status)
	}

	responseBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("body: %v", err)
	}

	var response RequestVoteResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("unmarshal: %v, raw body: %v", err, responseBody)
	}

	if !response.VoteGranted {
		return ErrVoteNotGranted
	}

	return nil
}

func (n *Node) Loop() {
	logging.GetLogger().Printf("Starting daemon. Node ID: %s", n.id)
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
		for i := range n.registry {
			wg.Add(1)

			node := &n.registry[i]
			go n.sendHeartbeat(node, wg)
		}
		wg.Wait()
	}
}

func (n *Node) Vote(request *RequestVoteRequest) error {
	switch n.state {
	case Follower:
		if n.votedFor != "" {
			return ErrAlreadyVoted
		}
	case Candidate:
		if n.votedFor != "" && n.votedFor != n.id {
			return ErrAlreadyVoted
		}
	case Leader:
		return ErrLeaderCantVote
	}

	if request.Term < n.currentTerm {
		return ErrTermOutdated
	}

	n.votedFor = request.CandidateID
	return nil
}

func (n *Node) HandleHeartbeat(r *AppendEntryRequest) error {
	logging.GetLogger().Printf("HEARTBEAT: [%s] Received", r.LeaderID)
	if n.currentTerm < r.Term {
		return ErrTermOutdated
	}
	n.heartbeat = true
	n.currentTerm = r.Term
	n.setState(Follower)
	return nil
}

func (n *Node) sendHeartbeat(node *Connection, wg *sync.WaitGroup) {
	defer wg.Done()

	nodeHost := fmt.Sprintf("%s:%s", node.url.Hostname(), node.url.Port())

	request := &AppendEntryRequest{
		Term:     n.currentTerm,
		LeaderID: n.id,
	}

	body, err := request.Marshal()
	if err != nil {
		logging.GetLogger().Printf("HEARTBEAT: [%s] Error sending heartbeat: %v", nodeHost, err)
		return
	}

	bodyReader := bytes.NewReader(body)
	response, err := http.Post(fmt.Sprintf("http://%s/AppendEntry", nodeHost), "application/json", bodyReader)
	if err != nil {
		logging.GetLogger().Printf("HEARTBEAT: [%s] Error sending heartbeat: %v", nodeHost, err)
		return
	} else if response.StatusCode != http.StatusOK {
		logging.GetLogger().Printf("HEARTBEAT: [%s] Error sending heartbeat: %s", nodeHost, response.Status)
		return
	}

	node.lastSeen = now()

	logging.GetLogger().Printf("HEARTBEAT: [%s] Heartbeat sent", nodeHost)
}

func (n *Node) setState(state NodeState) {
	if n.state != state {
		logging.GetLogger().Printf("STATE: New state: %s", state)
		n.state = state
	}
}

func now() *time.Time {
	t := time.Now()
	return &t
}
