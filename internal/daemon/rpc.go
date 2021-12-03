package daemon

import (
	"encoding/json"
)

type RequestVoteRequest struct {
	Term         int    `json:"term"`
	CandidateID  string `json:"candidateId"`
	LastLogIndex int    `json:"lastLogIndex"`
	LastLogTerm  int    `json:"lastLogTerm"`
}

func (r *RequestVoteRequest) Marshal() ([]byte, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

func (r *RequestVoteRequest) Unmarshal(raw []byte) error {
	return json.Unmarshal(raw, &r)
}

type RequestVoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"voteGranted"`
}

func (r *RequestVoteResponse) Marshal() ([]byte, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

func (r *RequestVoteResponse) Unmarshal(raw []byte) error {
	return json.Unmarshal(raw, &r)
}

type AppendEntryRequest struct {
	Term         int      `json:"term"`
	LeaderID     string   `json:"leaderId"`
	PrevLogIndex int      `json:"prevLogIndex"`
	PrevLogTerm  int      `json:"prevLogTerm"`
	Entries      []string `json:"entries"`
	LeaderCommit int      `json:"leaderCommit"`
}

func (r *AppendEntryRequest) Marshal() ([]byte, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

func (r *AppendEntryRequest) Unmarshal(raw []byte) error {
	return json.Unmarshal(raw, &r)
}

type AppendEntryResponse struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
}

func (r *AppendEntryResponse) Marshal() ([]byte, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

func (r *AppendEntryResponse) Unmarshal(raw []byte) error {
	return json.Unmarshal(raw, &r)
}
