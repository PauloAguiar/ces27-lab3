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

const (
	keepState string = "keepState"
	keepTerm int = -1
    keepVotedFor int = -1
	nullVote int = 0
)

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

// Updates internal state;
func (raft *Raft) update(state string, term int, votedFor int) {
    if state!=keepState {
        raft.currentState.Set(state)
    }
    if term!=keepTerm {
        raft.currentTerm = term
    }
    if votedFor!=keepVotedFor {
        raft.votedFor = votedFor
    }
}

// followerSelect implements the logic to handle messages from distinct
// events when in follower state.
func (raft *Raft) followerSelect() {
	log.Println("[FOLLOWER] Run Logic.")
	raft.resetElectionTimeout()
	for {
		select {
		case <-raft.electionTick:
			log.Println("[FOLLOWER] Election timeout. Starting new election.")
			raft.currentState.Set(candidate)
			return // Start new election; the election timeout will be reset when entering the candidate select;

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
            voteGranted := false
            if rv.Term>raft.currentTerm {
                log.Printf("[FOLLOWER] %v is running for a new term %v.\n", rv.CandidateID, rv.Term)
                log.Printf("[FOLLOWER] Vote granted to peer %v for term %v.\n", rv.CandidateID, rv.Term)
                voteGranted = true
                raft.update(keepState, rv.Term, rv.CandidateID)
            } else if rv.Term==raft.currentTerm && (raft.votedFor==nullVote || raft.votedFor==rv.CandidateID) { // Handle delayed/duplicated request vote;
                log.Printf("[FOLLOWER] Vote granted to peer %v for term %v.\n", rv.CandidateID, rv.Term)
                voteGranted = true
                raft.update(keepState, keepTerm, rv.CandidateID)
            } else {
                log.Printf("[FOLLOWER] Vote denied to peer %v for term %v.\n", rv.CandidateID, rv.Term)
            }
            reply := &RequestVoteReply{
				Term: raft.currentTerm,
                VoteGranted: voteGranted,
			}
			rv.replyChan <- reply
            if voteGranted {
                raft.resetElectionTimeout()
            }
			break
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
            success := false
            if ae.Term>=raft.currentTerm { // Valid append entry from current leader;
                if ae.Term>raft.currentTerm {
                    log.Printf("[FOLLOWER] %v is leading a new term %v.\n", ae.LeaderID, ae.Term)
                }
                log.Printf("[FOLLOWER] AppendEntry accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)
                success = true
                if ae.Term>raft.currentTerm {
                    raft.update(keepState, ae.Term, nullVote)
                }
            } else { // Invalid append entry from previous leaders;
                log.Printf("[FOLLOWER] AppendEntry rejected from peer %v for term %v.\n", ae.LeaderID, ae.Term)
            }
            reply := &AppendEntryReply{
                Term: raft.currentTerm,
                Success: success,
            }
			ae.replyChan <- reply
            if success {
                raft.resetElectionTimeout()
            }
			break
			// END OF MODIFY //
			///////////////////
		}
	}
}

// candidateSelect implements the logic to handle messages from distinct
// events when in candidate state.
func (raft *Raft) candidateSelect() {
    
	log.Println("[CANDIDATE] Run Logic.")
	// Candidates (§5.2):
	// Increment currentTerm, vote for self
	raft.currentTerm++
    majority := len(raft.peers)/2+1
    log.Printf("[CANDIDATE] Running for term %v.\n", raft.currentTerm)
	raft.votedFor = raft.me
	voteCount := 1
    log.Printf("[CANDIDATE] Vote granted by myself for term %v. Vote count: %v. Majority: %v.\n", raft.currentTerm, voteCount, majority)
	// Reset election timeout
	raft.resetElectionTimeout()
	// Send RequestVote RPCs to all other servers
	replyChan := make(chan *RequestVoteReply, 10*len(raft.peers))
	raft.broadcastRequestVote(replyChan)

	for {
		select {
		case <-raft.electionTick:
			// If election timeout elapses: start new election
			log.Println("[CANDIDATE] Election timeout. Starting new election.")
			raft.currentState.Set(candidate)
			return // Start new election; the election timeout will be reset when entering the candidate select;
		case rvr := <-replyChan:
			///////////////////
			//  MODIFY HERE  //
            if rvr.Term==raft.currentTerm && rvr.VoteGranted {
                voteCount++
                log.Printf("[CANDIDATE] Vote granted by peer %v for term %v. Vote count: %v. Majority: %v.\n", rvr.peerIndex, raft.currentTerm, voteCount, majority)
                if voteCount>=majority { // Check majority;
                    log.Printf("[CANDIDATE] Elected new leader for term %v.\n", raft.currentTerm)
                    raft.update(leader, keepTerm, keepVotedFor) // I am the new leader;
                    return // The election timeout will be reset when entering the leader select;
                }
            } else {
                log.Printf("[CANDIDATE] Vote denied by peer %v for term %v.\n", rvr.peerIndex, raft.currentTerm)
                if rvr.Term>raft.currentTerm {
                    log.Printf("[CANDIDATE] %v is running in a new term %v.\n", rvr.peerIndex, rvr.Term)
                    log.Printf("[CANDIDATE] Stepping down from term %v.\n", raft.currentTerm)
                    raft.update(follower, rvr.Term, nullVote) // Stepping down;
                    return // The election timeout will be reset when entering the follower select;
                }
            }
            break
			// END OF MODIFY //
			///////////////////

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
            voteGranted := false
            if rv.Term>raft.currentTerm {
                log.Printf("[CANDIDATE] %v is running for a new term %v.\n", rv.CandidateID, rv.Term)
                log.Printf("[CANDIDATE] Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[CANDIDATE] Vote granted to peer %v for term %v.\n", rv.CandidateID, rv.Term)
                voteGranted = true
                raft.update(follower, rv.Term, rv.CandidateID) // Stepping down;
            } else {
                log.Printf("[CANDIDATE] Vote denied to peer %v for term %v.\n", rv.CandidateID, rv.Term)
            }
            reply := &RequestVoteReply{
				Term: raft.currentTerm,
                VoteGranted: voteGranted,
			}
			rv.replyChan <- reply
            if voteGranted { // Became follower;
                return // The election timeout will be reset when entering the follower select;
            } else { // Still candidate;
                break
            }
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
            success := false
            if ae.Term>=raft.currentTerm { // New leader in place;
                if ae.Term>raft.currentTerm {
                    log.Printf("[CANDIDATE] %v is leading a new term %v.\n", ae.LeaderID, ae.Term)
                } else {
                    log.Printf("[CANDIDATE] %v was elected for term %v.\n", ae.LeaderID, ae.Term)
                }
                log.Printf("[CANDIDATE] Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[CANDIDATE] AppendEntry accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)
                success = true
                if ae.Term==raft.currentTerm { // New leader for the candidate's current term;
                    raft.update(follower, keepTerm, keepVotedFor) // Stepping down;
                } else { // New leader for a new term;
                    raft.update(follower, ae.Term, nullVote) // Stepping down;   
                }
                raft.appendEntryChan <- ae
            } else {
                log.Printf("[CANDIDATE] AppendEntry rejected from peer %v for term %v.\n", ae.LeaderID, ae.Term)
            }
            reply := &AppendEntryReply{
				Term: raft.currentTerm,
                Success: success,
			}
			ae.replyChan <- reply
			if success { // Became follower;
                return // The election timeout will be reset when entering the follower select;
            } else { // Still candidate;
                break
            }
			// END OF MODIFY //
			///////////////////
		}
	}
}

// leaderSelect implements the logic to handle messages from distinct
// events when in leader state.
func (raft *Raft) leaderSelect() {
	log.Println("[LEADER] Run Logic.")
    log.Printf("[LEADER] Starting term %v.\n", raft.currentTerm)
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
		case aer := <-replyChan:
			///////////////////
			//  MODIFY HERE  //
            if aer.Term>raft.currentTerm { // New leader in place;
                log.Printf("[LEADER] %v is running in a new term %v.\n", aer.peerIndex, aer.Term)
                log.Printf("[LEADER] Stepping down from term %v.\n", raft.currentTerm)
                raft.update(follower, aer.Term, nullVote) // Stepping down;
                return // The election timeout will be reset when entering the follower select;
            }
            break
			// END OF MODIFY //
			///////////////////
		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
            voteGranted := false
            if rv.Term>raft.currentTerm {
                log.Printf("[LEADER] %v is running for a new term %v.\n", rv.CandidateID, rv.Term)
                log.Printf("[LEADER] Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[LEADER] Vote granted to peer %v for term %v.\n", rv.CandidateID, rv.Term)
                voteGranted = true
                raft.update(follower, rv.Term, rv.CandidateID) // Stepping down;
            } else {
                log.Printf("[LEADER] Vote denied to peer %v for term %v.\n", rv.CandidateID, rv.Term)
            }
            reply := &RequestVoteReply{
				Term: raft.currentTerm,
                VoteGranted: voteGranted,
			}
            rv.replyChan <- reply
            if voteGranted { // Became follower;
                return // The election timeout will be reset when entering the follower select;
            } else { // Still leader;
                break
            }
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
            success := false
            if ae.Term>raft.currentTerm {
                log.Printf("[LEADER] %v is leading in a new term %v.\n", ae.LeaderID, ae.Term)
                log.Printf("[LEADER] Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[LEADER] AppendEntry accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)
                success = true
                raft.update(follower, ae.Term, nullVote) // Stepping down;
                raft.appendEntryChan <- ae
            } else if ae.Term==raft.currentTerm { // This should never happen!!
                log.Printf("[LEADER] AppendEntry denied from peer %v for term %v.\n", ae.LeaderID, ae.Term)
                panic(errors.New("[LEADER] Error: multiple leaders for current term.\n"))
            } else {
                log.Printf("[LEADER] AppendEntry denied from peer %v for term %v.\n", ae.LeaderID, ae.Term)
            }
            reply := &AppendEntryReply{
				Term: raft.currentTerm,
                Success: success,
			}
			ae.replyChan <- reply
            if success { // Became follower;
                return // The election timeout will be reset when entering the follower select;
            } else { // Still leader;
                break
            }
			// END OF MODIFY //
			///////////////////
		}
	}
}
