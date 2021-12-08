package daemon

import "fmt"

// ErrVoteNotGranted is returned when a RequestVoteResponse contains a false value for voteGranted.
type ErrVoteNotGranted struct{}

// Error implements the error interface.
func (ErrVoteNotGranted) Error() string {
	return "vote not granted"
}

// ErrTermOutdated is returned when a RequestVoteRequest has an older term than the node's current term.
type ErrTermOutdated struct {
	termReceived int
	termCurrent  int
}

// NewErrTermOutdated returns a new ErrTermOutdated value.
func NewErrTermOutdated(received, current int) ErrTermOutdated {
	return ErrTermOutdated{
		termReceived: received,
		termCurrent:  current,
	}
}

// Error implements the error interface.
func (e ErrTermOutdated) Error() string {
	return fmt.Sprintf("term received (%d) was earlier than current term (%d)", e.termReceived, e.termCurrent)
}

// ErrLeaderCantVote is returned when a leader receives a RequestVoteRequest.
type ErrLeaderCantVote struct{}

// Error implements the error interface.
func (ErrLeaderCantVote) Error() string {
	return "leader can't vote"
}

// ErrAlreadyVoted is returned when the node is requested to vote but has already voted in the current term.
type ErrAlreadyVoted struct{}

// Error implements the error interface.
func (ErrAlreadyVoted) Error() string {
	return "already voted"
}

// ErrElectionTimeout is returned when the election times out.
type ErrElectionTimeout struct{}

// Error implements the error interface.
func (ErrElectionTimeout) Error() string {
	return "election timed out"
}
