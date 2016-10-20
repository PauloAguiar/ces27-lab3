package raft

// AppendEntryArgs is invoked by leader to replicate log entries (§5.3); also used as
// heartbeat (§5.2).
// Term  	- leader’s term
// leaderId - so follower can redirect clients
type AppendEntryArgs struct {
	Term     int
	LeaderID int

	// internal
	replyChan chan *AppendEntryReply
}

// AppendEntryReply contains data to be returned to leader
// Term - currentTerm, for leader to update itself
// Success - true if follower contained entry matching PrevLogIndex and PrevLogTerm
type AppendEntryReply struct {
	Term    int
	Success bool

	// internal
	peerIndex int
}

// AppendEntry is called by other instances of Raft. It'll write the args received
// in the appendEntryChan.
func (rpc *RPC) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) error {
	args.replyChan = make(chan *AppendEntryReply)
	rpc.raft.appendEntryChan <- args
	*reply = *<-args.replyChan
	return nil
}

// broadcastAppendEntries will send AppendEntry to all peers
func (raft *Raft) broadcastAppendEntries(replyChan chan<- *AppendEntryReply) {
	args := &AppendEntryArgs{
		Term:     raft.currentTerm,
		LeaderID: raft.me,
	}

	for peerIndex := range raft.peers {
		if peerIndex != raft.me {
			go func(peer int) {
				reply := &AppendEntryReply{}
				ok := raft.sendAppendEntry(peer, args, reply)
				if ok {
					reply.peerIndex = peer
					replyChan <- reply
				}
			}(peerIndex)
		}
	}
}

// sendAppendEntry will send AppendEntry to a peer
func (raft *Raft) sendAppendEntry(peerIndex int, args *AppendEntryArgs, reply *AppendEntryReply) bool {
	err := raft.CallHost(peerIndex, "AppendEntry", args, reply)
	if err != nil {
		return false
	}
	return true
}
