package raft

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pauloaguiar/ces27-lab3/util"
)

// Raft is the struct that hold all information that is used by this instance
// of raft.
type Raft struct {
	sync.Mutex

	serv *server
	done chan struct{}

	peers map[int]string
	me    int

	// Persistent state on all servers:
	// currentTerm latest term server has seen (initialized to 0 on first boot, increases monotonically)
	// votedFor candidateId that received vote in current term (or 0 if none)
	currentState *util.ProtectedString
	currentTerm  int
	votedFor     int

	// Goroutine communication channels
	electionTick    <-chan time.Time
	requestVoteChan chan *RequestVoteArgs
	appendEntryChan chan *AppendEntryArgs
}

// NewRaft create a new raft object and return a pointer to it.
func NewRaft(peers map[int]string, me int) *Raft {
	var err error

	// 0 is reserved to represent undefined vote/leader
	if me == 0 {
		panic(errors.New("Reserved instanceID('0')"))
	}

	raft := &Raft{
		done: make(chan struct{}),

		peers: peers,
		me:    me,

		currentState: util.NewProtectedString(),
		currentTerm:  0,
		votedFor:     0,

		requestVoteChan: make(chan *RequestVoteArgs, 10*len(peers)),
		appendEntryChan: make(chan *AppendEntryArgs, 10*len(peers)),
	}

	raft.serv, err = newServer(raft, peers[me])
	if err != nil {
		panic(err)
	}

	go raft.loop()

	return raft
}

// Done returns a channel that will be used when the instance is done.
func (raft *Raft) Done() <-chan struct{} {
	return raft.done
}

// All changes to Raft structure should occur in the context of this routine.
// This way it's not necessary to use synchronizers to protect shared data.
// To send data to each of the states, use the channels provided.
func (raft *Raft) loop() {

	err := raft.serv.startListening()
	if err != nil {
		panic(err)
	}

	raft.currentState.Set(follower)
	for {
		switch raft.currentState.Get() {
		case follower:
			raft.followerSelect()
		case candidate:
			raft.candidateSelect()
		case leader:
			raft.leaderSelect()
		}
	}
}

// followerSelect implements the logic to handle messages from distinct
// events when in follower state.
func (raft *Raft) followerSelect() {
	log.Printf("[FOLLOWER %v] Run Logic.\n", raft.currentTerm)
	raft.resetElectionTimeout()
	for {
		select {
		case <-raft.electionTick:
			log.Printf("[FOLLOWER %v] Election timeout.\n", raft.currentTerm)
			raft.currentState.Set(candidate)
			return

		case rv := <-raft.requestVoteChan:
			///////////////////
			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}

			if rv.Term > raft.currentTerm {
				raft.currentTerm = rv.Term
				raft.votedFor = 0
				reply.Term = rv.Term
			}
			if rv.Term < raft.currentTerm {
				break
			}

			if raft.votedFor != 0 {
				log.Printf("[FOLLOWER %v] Vote denied to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = false
			} else {
				log.Printf("[FOLLOWER %v] Vote granted to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				raft.votedFor = rv.CandidateID
				reply.VoteGranted = true
			}
			raft.resetElectionTimeout()
			rv.replyChan <- reply
			break
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			if ae.Term > raft.currentTerm {
				raft.currentTerm = ae.Term
				raft.votedFor = 0
				reply.Term = ae.Term
			}

			if ae.Term < raft.currentTerm {
				//log.Printf("[FOLLOWER %v] Reject AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = false
			} else {
				//log.Printf("[FOLLOWER %v] Accept AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = true
			}

			raft.resetElectionTimeout()
			ae.replyChan <- reply
			break
			///////////////////
		}
	}
}

// candidateSelect implements the logic to handle messages from distinct
// events when in candidate state.
func (raft *Raft) candidateSelect() {
	log.Printf("[CANDIDATE %v] Run Logic.\n", raft.currentTerm)
	// Candidates (§5.2):
	// Increment currentTerm, vote for self
	raft.currentTerm++
	raft.votedFor = raft.me
	voteCount := 1

	log.Printf("[CANDIDATE %v] Running for term '%v'.\n", raft.currentTerm, raft.currentTerm)
	// Reset election timeout
	raft.resetElectionTimeout()
	// Send RequestVote RPCs to all other servers
	replyChan := make(chan *RequestVoteReply, 10*len(raft.peers))
	raft.broadcastRequestVote(replyChan)

	for {
		select {
		case <-raft.electionTick:
			// If election timeout elapses: start new election
			log.Printf("[CANDIDATE %v] Election timeout.\n", raft.currentTerm)
			raft.currentState.Set(candidate)
			return
		case rvr := <-replyChan:
			///////////////////

			if rvr.Term > raft.currentTerm {
				raft.currentTerm = rvr.Term
				raft.votedFor = 0
				raft.resetElectionTimeout()
			}
			if rvr.Term < raft.currentTerm {
				break
			}

			if rvr.VoteGranted {
				log.Printf("[CANDIDATE %v] Vote granted by '%v'.\n", raft.currentTerm, raft.peers[rvr.peerIndex])
				voteCount++
			} else {
				log.Printf("[CANDIDATE %v] Vote denied by '%v'.\n", raft.currentTerm, raft.peers[rvr.peerIndex])
			}
			log.Printf("[CANDIDATE %v] VoteCount: %v\n", raft.currentTerm, voteCount)

			if 2*voteCount > 5 {
				raft.currentState.Set(leader)
				return
			}
			raft.resetElectionTimeout()

			///////////////////

		case rv := <-raft.requestVoteChan:
			///////////////////

			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}

			if rv.Term > raft.currentTerm {
				log.Printf("[CANDIDATE %v] Vote granted to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = true
				raft.resetElectionTimeout()
				rv.replyChan <- reply
				raft.currentTerm = rv.Term
				reply.Term = rv.Term
				raft.votedFor = rv.CandidateID
				raft.currentState.Set(follower)
				return
			} else {
				log.Printf("[CANDIDATE %v] Vote denied to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = false
				raft.resetElectionTimeout()
				rv.replyChan <- reply
				break
			}

			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////

			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			if ae.Term < raft.currentTerm {
				//log.Printf("[CANDIDATE %v] Reject AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = false
				raft.resetElectionTimeout()
				ae.replyChan <- reply
				break
			} else {
				log.Printf("[CANDIDATE %v] Accept AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = true
				raft.currentState.Set(follower)
				if ae.Term > raft.currentTerm {
					raft.currentTerm = ae.Term
					reply.Term = ae.Term
					raft.votedFor = 0
				}
				raft.resetElectionTimeout()
				ae.replyChan <- reply
				return
			}

			///////////////////
		}
	}
}

// leaderSelect implements the logic to handle messages from distinct
// events when in leader state.
func (raft *Raft) leaderSelect() {
	log.Printf("[LEADER %v] Run Logic.\n", raft.currentTerm)
	replyChan := make(chan *AppendEntryReply, 10*len(raft.peers))
	raft.broadcastAppendEntries(replyChan)

	heartbeat := time.NewTicker(raft.broadcastInterval())
	defer heartbeat.Stop()

	broadcastTick := make(chan time.Time)
	defer close(broadcastTick)

	go func() {
		for t := range heartbeat.C {
			broadcastTick <- t
		}
	}()

	for {
		select {
		case <-broadcastTick:
			raft.broadcastAppendEntries(replyChan)
		case aet := <-replyChan:
			///////////////////

			//log.Printf("[LEADER %v] Ack received from '%v' for term '%v'.\n", raft.currentTerm, raft.peers[aet.peerIndex], raft.currentTerm)
			if aet.Term > raft.currentTerm {
				raft.currentTerm = aet.Term
				raft.votedFor = 0
			}
			if aet.Term < raft.currentTerm {
				break
			}

			///////////////////
		case rv := <-raft.requestVoteChan:
			///////////////////

			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}

			if rv.Term < raft.currentTerm {
				log.Printf("[LEADER %v] Vote denied to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = false
				rv.replyChan <- reply
				break
			} else if rv.Term == raft.currentTerm {
				log.Printf("[LEADER %v] Vote denied to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = false
				rv.replyChan <- reply
				raft.currentState.Set(candidate)
				return
			} else {
				log.Printf("[LEADER %v] Vote granted to '%v' for term '%v'.\n", raft.currentTerm, raft.peers[rv.CandidateID], rv.Term)
				reply.VoteGranted = true
				raft.currentTerm = rv.Term
				raft.votedFor = rv.CandidateID
				reply.Term = rv.Term
				rv.replyChan <- reply
				raft.currentState.Set(follower)
				return
			}

			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			if ae.Term < raft.currentTerm {
				//log.Printf("[LEADER %v] Reject AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = false
				ae.replyChan <- reply
				break
			} else if ae.Term == raft.currentTerm {
				//log.Printf("[LEADER %v] Reject AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = false
				ae.replyChan <- reply
				raft.currentState.Set(candidate)
				return
			} else {
				//log.Printf("[LEADER %v] Accept AppendEntry from '%v'.\n", raft.currentTerm, raft.peers[ae.LeaderID])
				reply.Success = true
				raft.currentTerm = ae.Term
				raft.votedFor = 0
				reply.Term = ae.Term
				ae.replyChan <- reply
				raft.currentState.Set(follower)
				return
			}

			///////////////////
		}
	}
}
