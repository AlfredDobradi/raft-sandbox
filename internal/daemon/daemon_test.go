package daemon

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

type MockNode struct {
	mu      sync.Mutex
	Success bool
	Calls   map[string]int
}

func NewMockNode(success bool) *MockNode {
	return &MockNode{
		mu:      sync.Mutex{},
		Success: success,
		Calls:   make(map[string]int),
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
	response := &RequestVoteResponse{
		Term:        1,
		VoteGranted: n.Success,
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

func (s *Suite) Setup(hosts map[string]bool, callHosts map[string]string) error {
	logging.SetLogger(log.New(os.Stdout, "TEST ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile))

	s.Daemon = &Node{
		term:   1,
		state:  Follower,
		errors: make(chan error),
	}

	s.Servers = make(map[string]*http.Server)

	for host, success := range hosts {
		u, err := url.Parse(host)
		if err != nil {
			return fmt.Errorf("error parsing hostname: %v", err)
		}
		uu := fmt.Sprintf("%s:%s", u.Hostname(), u.Port())
		s.Servers[uu] = &http.Server{
			Addr:    uu,
			Handler: NewMockNode(success),
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
		hosts            map[string]bool
		callHosts        map[string]string
		callExpectations map[string]map[string]int
		expectedErrors   []error
		newState         NodeState
	}{
		{
			label: "elected",
			hosts: map[string]bool{
				"http://localhost:63000": true,
				"http://localhost:63001": true,
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
			hosts: map[string]bool{
				"http://localhost:63000": false,
				"http://localhost:63001": false,
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
			hosts: map[string]bool{
				"http://localhost:63000": false,
				"http://localhost:63001": false,
			},
			callHosts: map[string]string{
				"http://localhost:63000": "http://localhost:63005",
				"http://localhost:63001": "http://localhost:63006",
			},
			callExpectations: map[string]map[string]int{},
			newState:         Follower,
			expectedErrors: []error{
				fmt.Errorf(`ELECTION: [localhost:63006] Error getting vote: post: Post "http://localhost:63006/RequestVote": dial tcp [::1]:63006: connect: connection refused`),
				fmt.Errorf(`ELECTION: [localhost:63005] Error getting vote: post: Post "http://localhost:63005/RequestVote": dial tcp [::1]:63005: connect: connection refused`),
			},
		},
	}

	for i, tt := range tests {
		t.Logf("Test %d - %s", i+1, tt.label)
		tf := func(t *testing.T) {
			suite := NewSuite(t)
			defer suite.TearDown()
			if err := suite.Setup(tt.hosts, tt.callHosts); err != nil {
				t.Fatalf("error setting up suite: %v", err)
			}

			if expected, actual := tt.newState, suite.Daemon.state; expected != actual {
				suite.T.Logf("FAIL: Expected node state %s, got %s", expected, actual)
			}

			start := time.Now()
			logging.GetLogger().Printf("TEST: Starting election at %s", start.Format(time.RFC1123))
			suite.Daemon.Election()

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

				for _, expected := range tt.expectedErrors {
					found := false
					for _, actual := range suite.Errors {
						if expected.Error() == actual.Error() {
							found = true
						}
					}

					if !found {
						suite.T.Logf("FAIL: Expected error %s but never received it.", expected.Error())
					}
				}
			}

			suite.T.Log("PASS")
		}

		t.Run(tt.label, tf)
	}
}
