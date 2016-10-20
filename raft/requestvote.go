package raft

// RequestVoteArgs is the struct that hold data passed to
// RequestVote RPC calls.
type RequestVoteArgs struct {
	Term        int
	CandidateID int

	// internal
	replyChan chan *RequestVoteReply
}

// RequestVoteReply is the struct that hold data returned by
// RequestVote RPC calls.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool

	// internal
	peerIndex int
}

// RequestVote is called by other instances of Raft. It'll write the args received
// in the requestVoteChan.
func (rpc *RPC) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	args.replyChan = make(chan *RequestVoteReply)
	rpc.raft.requestVoteChan <- args
	*reply = *<-args.replyChan
	return nil
}

// broadcastRequestVote will send RequestVote to all peers
func (raft *Raft) broadcastRequestVote(replyChan chan<- *RequestVoteReply) {
	args := &RequestVoteArgs{
		CandidateID: raft.me,
		Term:        raft.currentTerm,
	}

	for peerIndex := range raft.peers {
		if peerIndex != raft.me { // exclude self
			go func(peer int) {
				reply := &RequestVoteReply{}
				ok := raft.sendRequestVote(peer, args, reply)
				if ok {
					reply.peerIndex = peer
					replyChan <- reply
				}
			}(peerIndex)
		}
	}
}

// sendRequestVote will send RequestVote to a peer
func (raft *Raft) sendRequestVote(peerIndex int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	err := raft.CallHost(peerIndex, "RequestVote", args, reply)
	if err != nil {
		return false
	}
	return true
}
