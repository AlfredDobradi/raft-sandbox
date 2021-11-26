package daemon

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

func (n *Node) Listen() {
	if n.listener == nil {
		m := mux.NewRouter()
		m.HandleFunc("/RequestVote", n.handleRequestVote).Methods(http.MethodPost)
		m.HandleFunc("/AppendEntry", n.handleAppendEntry).Methods(http.MethodPost)

		n.listener = &http.Server{
			Addr:         fmt.Sprintf("%s:%s", n.hostname, n.port),
			Handler:      m,
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}
	}

	log.Printf("HTTP service listening on %s", n.listener.Addr)
	if err := n.listener.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		n.errors <- err
	}
}

func (n *Node) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	var request RequestVoteRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logging.GetLogger().Printf("Request: %+v", request)

	voteGranted := false
	if err := n.Vote(request.CandidateID); err != nil && err != ErrAlreadyVoted {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	} else if err == nil {
		voteGranted = true
	}

	response := RequestVoteResponse{
		Term:        n.term,
		VoteGranted: voteGranted,
	}

	logging.GetLogger().Printf("Response: %+v", response)

	responseBody, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(responseBody) // nolint
}

// TODO proper request and response
func (n *Node) handleAppendEntry(w http.ResponseWriter, r *http.Request) {
	n.HandleHeartbeat()
	w.Write([]byte("append_entry")) // nolint
}
