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
func (raft *Raft) followerSelect() {  //raft is the follower 
	log.Println("[FOLLOWER] Run Logic.")
	raft.resetElectionTimeout()
	for {
		select {
		case <-raft.electionTick:
			log.Println("[FOLLOWER] Election timeout.")
			raft.currentState.Set(candidate)
			return

		case rv := <-raft.requestVoteChan: //candidate requests vote    (rv is the request with Term and CandidateID)
			///////////////////
			//  MODIFY HERE  //
			//-------------------------------------------------------------
			var grant bool;
			if rv.Term>raft.currentTerm{
				raft.currentTerm=rv.Term
				raft.votedFor=rv.CandidateID
				grant=true
				raft.resetElectionTimeout()
				log.Printf("[FOLLOWER] Vote to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			} else {   
				rv.Term=raft.currentTerm
				grant = false
				log.Printf("[FOLLOWER] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			}			
			//-------------------------------------------------------------
			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}
			reply.VoteGranted = grant
			rv.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan: //It is only the leader sending heartbeat or it is a request for entry replication, the follower answers and the leader asks for entry commit
			///////////////////
			//  MODIFY HERE  //			
			//-------------------------------------------------------------
			var grant bool;
			if(ae.Term==raft.currentTerm){
				grant=true;
				raft.resetElectionTimeout()
				log.Printf("[FOLLOWER] Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			}else { 
				raft.currentTerm=ae.Term				
				raft.votedFor=0
				grant=false;
				log.Printf("[FOLLOWER] Deny AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			}
			//-------------------------------------------------------------

			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			reply.Success = grant
			ae.replyChan <- reply
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
	raft.votedFor = raft.me
	voteCount := 1

	log.Printf("[CANDIDATE] Running for term '%v'.\n", raft.currentTerm)
	// Reset election timeout
	raft.resetElectionTimeout()
	// Send RequestVote RPCs to all other servers
	replyChan := make(chan *RequestVoteReply, 10*len(raft.peers))
	raft.broadcastRequestVote(replyChan)

	for {
		select {
		case <-raft.electionTick:
			// If election timeout elapses: start new election
			log.Println("[CANDIDATE] Election timeout.")
			raft.currentState.Set(candidate)
			return
		case rvr := <-replyChan:  //reply arrived in candidate
			///////////////////
			//  MODIFY HERE  //
			//-------------------------------------------------------------
			if rvr.Term>raft.currentTerm{
				raft.currentTerm=rvr.Term;				
				raft.votedFor=0
				log.Printf("[CANDIDATE] Stepping down.\n")
			    raft.currentState.Set(follower)
			    return
			}
			if voteCount>=len(raft.peers)/2{
				log.Println("[CANDIDATE] Majority of the votes, becoming leader.")
				raft.currentState.Set(leader)
				return
			}

			//-------------------------------------------------------------			
			if rvr.VoteGranted {
				log.Printf("[CANDIDATE] Vote granted by '%v'.\n", raft.peers[rvr.peerIndex])
				voteCount++
				log.Println("[CANDIDATE] VoteCount: ", voteCount)
				break
			}
			log.Printf("[CANDIDATE] Vote denied by '%v'.\n", raft.peers[rvr.peerIndex])

			// END OF MODIFY //
			///////////////////

		case rv := <-raft.requestVoteChan: //A vote request rv arrives in candidate
			///////////////////
			//  MODIFY HERE  //
			//-------------------------------------------------------------
			var grant bool;
			if rv.Term>raft.currentTerm{
				raft.currentTerm=rv.Term
				raft.votedFor=rv.CandidateID
				grant=true
				raft.currentState.Set(follower)
				raft.resetElectionTimeout()				
				log.Printf("[CANDIDATE] Vote to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			} else {   
				rv.Term=raft.currentTerm
				grant = false
				log.Printf("[CANDIDATE] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			}	
			//-------------------------------------------------------------
			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}
			//log.Printf("[CANDIDATE] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			reply.VoteGranted = grant
			rv.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			//--------------------------------------------------------------
			var grant bool;
			if(ae.Term>=raft.currentTerm){
				grant=true;
				raft.resetElectionTimeout()
				raft.currentState.Set(follower)
				log.Printf("[CANDIDATE] Stepping down.\n")
				raft.appendEntryChan <- ae
				return
			}else { 
				raft.currentTerm=ae.Term
				raft.votedFor=0
				grant=false
				log.Printf("[CANDIDATE] Deny AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			}

			//--------------------------------------------------------------

			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			//log.Printf("[CANDIDATE] Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			reply.Success = grant
			ae.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////
		}
	}
}

// leaderSelect implements the logic to handle messages from distinct
// events when in leader state.
func (raft *Raft) leaderSelect() {
	log.Println("[LEADER] Run Logic.")
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
			//  MODIFY HERE  //
			//-------------------------------------------------------------
			if aet.Term>raft.currentTerm{
				raft.currentTerm=aet.Term;				
				raft.votedFor=0
				log.Printf("[LEADER] Stepping down.\n")
			    raft.currentState.Set(follower)
			    replyChan <-aet
			    return
			}
			//-------------------------------------------------------------
			//_ = aet
			// END OF MODIFY //
			///////////////////
		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
			//-------------------------------------------------------------
			var grant bool;
			if rv.Term>raft.currentTerm{
				raft.currentTerm=rv.Term
				raft.currentState.Set(follower)
				raft.votedFor=rv.CandidateID
				grant=true
				raft.resetElectionTimeout()
				log.Printf("[LEADER] Stepping down.\n")
				raft.requestVoteChan <-rv
				return
			} else {   
				rv.Term=raft.currentTerm
				grant = false
				log.Printf("[LEADER] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			}	
			//-------------------------------------------------------------
			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}

			//log.Printf("[LEADER] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
			reply.VoteGranted = grant
			rv.replyChan <- reply
			break

			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			//--------------------------------------------------------------
			var grant bool;
			if(ae.Term==raft.currentTerm){
				grant=true;
				raft.resetElectionTimeout()
				log.Printf("[LEADER] Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			}else { 
				raft.currentTerm=ae.Term
				raft.votedFor=0
				grant=false;
				log.Printf("[LEADER] Deny AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			}
			//--------------------------------------------------------------
			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			//log.Printf("[LEADER] Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
			reply.Success = grant
			ae.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////
		}
	}
}
