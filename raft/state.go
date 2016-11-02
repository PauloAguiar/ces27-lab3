package raft

const (
	minElectionTimeoutMilli int = 500
	maxElectionTimeoutMilli int = 5000
)

const (
	follower  string = "Follower"
	candidate string = "Candidate"
	leader    string = "Leader"
)

// resetState will reset the state of a node when new term is discovered.
func (raft *Raft) resetState(term int) {
	raft.currentTerm = term
	raft.votedFor = 0
	raft.currentState.Set(follower)
}
