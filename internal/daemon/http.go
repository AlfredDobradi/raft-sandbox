package daemon

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func (n *Node) Listen() {
	if n.listener == nil {
		m := mux.NewRouter()
		m.HandleFunc("/RequestVote", n.handleRequestVote).Methods(http.MethodGet)
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
	if err := n.Vote(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Write([]byte("OK")) // nolint
}

func (n *Node) handleAppendEntry(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("append_entry")) // nolint
}
