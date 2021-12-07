package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

type hostOpts struct {
	success     bool
	timeout     time.Duration
	currentTerm int
}

type MockNode struct {
	mu    sync.Mutex
	Opts  hostOpts
	Calls map[string]int
}

func NewMockNode(options hostOpts) *MockNode {
	return &MockNode{
		mu:    sync.Mutex{},
		Opts:  options,
		Calls: make(map[string]int),
	}
}

func (n *MockNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := fmt.Sprintf("%s:%s", r.Method, r.URL.String())
	n.mu.Lock()
	if calls, ok := n.Calls[key]; !ok {
		n.Calls[key] = 1
	} else {
		n.Calls[key] = calls + 1
	}
	n.mu.Unlock()

	if r.URL.String() == "/RequestVote" && r.Method == http.MethodPost {
		n.requestVote(w, r)
	}
}

func (n *MockNode) requestVote(w http.ResponseWriter, r *http.Request) {
	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	request := &RequestVoteRequest{}
	if err := request.Unmarshal(requestBody); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if n.Opts.timeout > 0 {
		time.Sleep(n.Opts.timeout)
	}

	voteGranted := true
	if !n.Opts.success || request.Term < n.Opts.currentTerm {
		voteGranted = false
	}

	response := &RequestVoteResponse{
		Term:        1,
		VoteGranted: voteGranted,
	}

	responseBody, err := response.Marshal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBody) // nolint
}

type Suite struct {
	T       *testing.T
	Daemon  *Node
	Servers map[string]*http.Server
	Errors  []error
}

func NewSuite(t *testing.T) *Suite {
	return &Suite{T: t}
}

func (s *Suite) Setup(hosts map[string]hostOpts, callHosts map[string]string) error {
	logging.SetLogger(log.New(os.Stdout, "TEST ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile))

	s.Daemon = &Node{
		currentTerm: 1,
		state:       Follower,
		errors:      make(chan error),
	}

	s.Servers = make(map[string]*http.Server)

	for host, options := range hosts {
		u, err := url.Parse(host)
		if err != nil {
			return fmt.Errorf("error parsing hostname: %v", err)
		}
		uu := fmt.Sprintf("%s:%s", u.Hostname(), u.Port())
		s.Servers[uu] = &http.Server{
			Addr:    uu,
			Handler: NewMockNode(options),
		}

		if callHost, ok := callHosts[host]; ok {
			u, err = url.Parse(callHost)
			if err != nil {
				return fmt.Errorf("error parsing call hostname: %v", err)
			}
		}
		s.Daemon.registry = append(s.Daemon.registry, Connection{url: u})

	}

	for _, node := range s.Servers {
		go func(n *http.Server) {
			n.ListenAndServe() // nolint
		}(node)
	}

	go func() {
		for err := range s.Daemon.errors {
			s.Errors = append(s.Errors, err)
		}
	}()

	return nil
}

func (s *Suite) TearDown() {
	for _, node := range s.Servers {
		node.Shutdown(context.Background()) // nolint
	}
}

func TestElection(t *testing.T) {
	tests := []struct {
		label            string
		hosts            map[string]hostOpts
		callHosts        map[string]string
		callExpectations map[string]map[string]int
		expectedErrors   []string
		newState         NodeState
	}{
		{
			label: "elected",
			hosts: map[string]hostOpts{
				"http://localhost:63000": {
					success: true,
				},
				"http://localhost:63001": {
					success: true,
				},
			},
			callExpectations: map[string]map[string]int{
				"http://localhost:63000": {
					"POST:/AppendEntry": 1,
					"POST:/RequestVote": 1,
				},
				"http://localhost:63001": {
					"POST:/AppendEntry": 1,
					"POST:/RequestVote": 1,
				},
			},
			newState: Leader,
		},
		{
			label: "not_elected",
			hosts: map[string]hostOpts{
				"http://localhost:63000": {
					success: false,
				},
				"http://localhost:63001": {
					success: false,
				},
			},
			callExpectations: map[string]map[string]int{
				"http://localhost:63000": {
					"POST:/RequestVote": 1,
				},
				"http://localhost:63001": {
					"POST:/RequestVote": 1,
				},
			},
			newState: Follower,
		},
		{
			label: "errors",
			hosts: map[string]hostOpts{
				"http://localhost:63000": {
					success: false,
				},
				"http://localhost:63001": {
					success: false,
				},
			},
			callHosts: map[string]string{
				"http://localhost:63000": "http://localhost:63005",
				"http://localhost:63001": "http://localhost:63006",
			},
			callExpectations: map[string]map[string]int{},
			newState:         Follower,
			expectedErrors: []string{
				`Error getting vote: post: Post "http://localhost:63006/RequestVote": dial tcp [::1]:63006: connect: connection refused`,
				`Error getting vote: post: Post "http://localhost:63005/RequestVote": dial tcp [::1]:63005: connect: connection refused`,
			},
		},
		{
			label: "reject_lower_term",
			hosts: map[string]hostOpts{
				"http://localhost:63000": {
					success:     true,
					currentTerm: 2,
				},
				"http://localhost:63001": {
					success: true,
				},
			},
			callExpectations: map[string]map[string]int{},
			newState:         Leader,
			expectedErrors: []string{
				"Error getting vote: vote not granted",
			},
		},
		{
			label: "election_timeout",
			hosts: map[string]hostOpts{
				"http://localhost:63000": {
					success: true,
					timeout: 2 * time.Second,
				},
				"http://localhost:63001": {
					success: true,
					timeout: 2 * time.Second,
				},
			},
			callExpectations: map[string]map[string]int{},
			newState:         Follower,
			expectedErrors: []string{
				`Error getting vote: post: Post "http://localhost:63000/RequestVote": context deadline exceeded`,
				`Error getting vote: post: Post "http://localhost:63001/RequestVote": context deadline exceeded`,
			},
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			suite := NewSuite(t)
			defer suite.TearDown()
			if err := suite.Setup(tt.hosts, tt.callHosts); err != nil {
				t.Fatalf("error setting up suite: %v", err)
			}

			start := time.Now()
			logging.GetLogger().Printf("TEST: Starting election at %s", start.Format(time.RFC1123))
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			if err := suite.Daemon.Election(ctx, 1*time.Second); err != nil {
				cancel()
				suite.T.Fatalf("ERROR: %v", err)
			}
			cancel()

			time.Sleep(100 * time.Millisecond) // TODO: A nicer way to wait a bit for expected errors from channel

			if expected, actual := tt.newState, suite.Daemon.state; expected != actual {
				suite.T.Fatalf("FAIL: Expected node state %s, got %s", expected, actual)
			}

			for _, conn := range suite.Daemon.registry {
				if conn.lastSeen == nil && tt.newState == Leader {
					suite.T.Fatalf("FAIL: No heartbeat response found for %s", conn.url.String())
				}

				if conn.lastSeen != nil && !conn.lastSeen.After(start) {
					suite.T.Fatalf("FAIL: Heartbeat response (%s) earlier than start time (%s)", conn.lastSeen.Format(time.RFC1123), start.Format(time.RFC1123))
				}

				nodes := map[string]*MockNode{}
				for _, server := range suite.Servers {
					if node, ok := server.Handler.(*MockNode); !ok {
						suite.T.Fatalf("ERROR: Server %s isn't *MockNode type [%T]", server.Addr, node)
					} else {
						id := fmt.Sprintf("http://%s", server.Addr)
						nodes[id] = node
					}
				}
				for nodeID, expectations := range tt.callExpectations {
					node := nodes[nodeID]
					for route, num := range expectations {
						if expected, actual := num, node.Calls[route]; expected != actual {
							suite.T.Fatalf("FAIL: Route %s expected %d calls, got %d", route, expected, actual)
						}
					}
				}

				logging.GetLogger().Printf("%+v", suite.Errors)

				for _, expected := range tt.expectedErrors {
					found := false
					for _, actual := range suite.Errors {
						if strings.Contains(actual.Error(), expected) {
							found = true
						}
					}

					if !found {
						suite.T.Fatalf("FAIL: Expected error %s but never received it.", expected)
					}
				}
			}

			suite.T.Log("PASS")
		}

		t.Run(tt.label, tf)
	}
}

func TestVote(t *testing.T) {
	tests := []struct {
		label            string
		request          *RequestVoteRequest
		daemonTerm       int
		daemonState      NodeState
		daemonID         string
		votedFor         string
		expectedResult   error
		expectedVotedFor string
	}{
		{
			label:            "successful_vote",
			request:          &RequestVoteRequest{Term: 2, CandidateID: "node_1"},
			daemonTerm:       1,
			daemonState:      Follower,
			daemonID:         "node_0",
			votedFor:         "",
			expectedResult:   nil,
			expectedVotedFor: "node_1",
		},
		{
			label:            "outdated_term",
			request:          &RequestVoteRequest{Term: 1, CandidateID: "node_1"},
			daemonTerm:       2,
			daemonState:      Follower,
			daemonID:         "node_0",
			votedFor:         "",
			expectedResult:   ErrTermOutdated,
			expectedVotedFor: "",
		},
		{
			label:            "follower_already_voted",
			request:          &RequestVoteRequest{Term: 2, CandidateID: "node_1"},
			daemonTerm:       1,
			daemonState:      Follower,
			daemonID:         "node_0",
			votedFor:         "node_2",
			expectedResult:   ErrAlreadyVoted,
			expectedVotedFor: "node_2",
		},
		{
			label:            "candidate_already_voted",
			request:          &RequestVoteRequest{Term: 2, CandidateID: "node_1"},
			daemonTerm:       1,
			daemonState:      Candidate,
			daemonID:         "node_0",
			votedFor:         "node_2",
			expectedResult:   ErrAlreadyVoted,
			expectedVotedFor: "node_2",
		},
		{
			label:            "leader_cant_vote",
			request:          &RequestVoteRequest{Term: 2, CandidateID: "node_1"},
			daemonTerm:       1,
			daemonState:      Leader,
			daemonID:         "node_0",
			votedFor:         "",
			expectedResult:   ErrLeaderCantVote,
			expectedVotedFor: "",
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			d := &Node{
				currentTerm: tt.daemonTerm,
				votedFor:    tt.votedFor,
				state:       tt.daemonState,
			}

			err := d.Vote(tt.request)
			if tt.expectedResult != nil {
				if err == nil {
					t.Fatalf("FAIL: Expected error %s, got no error", tt.expectedResult.Error())
				}

				if err.Error() != tt.expectedResult.Error() {
					t.Fatalf("FAIL: Expected error %s, got %s", tt.expectedResult.Error(), err.Error())
				}
			} else if err != nil {
				t.Fatalf("FAIL: Expected no errors, got %s", err.Error())
			}

			if expected, actual := tt.expectedVotedFor, d.votedFor; expected != actual {
				t.Fatalf("FAIL: Expected node to vote for %s, voted for %s", expected, actual)
			}
		}

		t.Run(tt.label, tf)
	}
}
