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
	currentState	*util.ProtectedString
	currentTerm 	int
	votedFor    	int

	// Goroutine communication channels
	electionTick    <-chan time.Time
	requestVoteChan chan *RequestVoteArgs
	appendEntryChan chan *AppendEntryArgs
}


///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

// Constantes para toda a aplicação
const (
	nullVote int = 0
	updatedTerm int = -1
    updatedVotedFor int = -1
	updatedState string = "updatedState"
)

// Variável para toda a aplicação
var currentLeaderID	= 0

// Atualizar estados
func (raft *Raft) updateStates(votedFor int, term int, state string) {
    if votedFor != updatedVotedFor {
        raft.votedFor = votedFor
    }

    if term != updatedTerm {
        raft.currentTerm = term
    }

    if state != updatedState && (state == leader || state == candidate || state == follower) { 
        raft.currentState.Set(state)
    }
}

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

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
	log.Println("[FOLLOWER] Run Logic.")
	raft.resetElectionTimeout()
	for {
		select {
		case <-raft.electionTick:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

			// New election
			log.Println("[FOLLOWER] Election failed: timeout! New election is starting now.")

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

			raft.currentState.Set(candidate)
			return

		case rv := <-raft.requestVoteChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

            vote := false

            if rv.Term == raft.currentTerm && (raft.votedFor == rv.CandidateID || raft.votedFor == nullVote) { 

                vote = true
                raft.updateStates(rv.CandidateID, updatedTerm, updatedState)
                log.Printf("[FOLLOWER] There is a vote to peer %v for term %v.\n", rv.CandidateID, rv.Term)

            } else if rv.Term > raft.currentTerm {

                vote = true
                raft.updateStates(rv.CandidateID, rv.Term, updatedState)
                log.Printf("[FOLLOWER] There is a vote to peer %v for term %v.\n", rv.CandidateID, rv.Term)

            } else {

                log.Printf("[FOLLOWER] I refused to vote to peer %v for term %v.\n", rv.CandidateID, rv.Term)

            }

            replyVote := &RequestVoteReply{
                VoteGranted: vote,
				Term: raft.currentTerm,
			}

			rv.replyChan <- replyVote

            if vote {
                raft.resetElectionTimeout()
            }

			break

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		case ae := <-raft.appendEntryChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

            ok := false

            if ae.Term > raft.currentTerm {

                ok = true
				if currentLeaderID != ae.LeaderID { 
					log.Printf("[FOLLOWER] Warning! Leader %v has elected to a new term %v.\n", ae.LeaderID, ae.Term)
				}
				currentLeaderID = ae.LeaderID
                raft.updateStates(nullVote, ae.Term, updatedState)

			} else if ae.Term == raft.currentTerm {

                ok = true
				if currentLeaderID != ae.LeaderID { 
					log.Printf("[FOLLOWER] Warning! Leader %v has elected to a new term %v.\n", ae.LeaderID, ae.Term)
				}
				currentLeaderID = ae.LeaderID

            } else {

                log.Printf("[FOLLOWER] AppendEntry has rejected to peer %v for term %v.\n", ae.LeaderID, ae.Term)

            }

            replyVote := &AppendEntryReply{
                Success: ok,
                Term: raft.currentTerm,
            }

			ae.replyChan <- replyVote

            if ok {
                raft.resetElectionTimeout()
            }

			break

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

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

///////////////////////////////////t
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

    amountToElected := (len(raft.peers) / 2) + 1

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

    log.Printf("[CANDIDATE] Running for term %v.\n", raft.currentTerm)
	// Reset Election failed: timeout
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
		case rvr := <-replyChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

			if (rvr.VoteGranted && rvr.Term == raft.currentTerm && voteCount >= amountToElected) {

                voteCount++
				raft.updateStates(updatedVotedFor, updatedTerm, leader)
				log.Printf("[CANDIDATE] A new leader has elected for term %v.\n", raft.currentTerm)
				return

            } else if (rvr.VoteGranted && rvr.Term == raft.currentTerm) {

                voteCount++
				log.Printf("[CANDIDATE] There was a vote to peer %v. Actual vote score: %v.\n", rvr.peerIndex, voteCount)

			} else if rvr.Term > raft.currentTerm {

				raft.updateStates(nullVote, rvr.Term, follower)
				log.Printf("[CANDIDATE] This vote has denied by peer %v for term %v.\n", rvr.peerIndex, raft.currentTerm)
				log.Printf("[CANDIDATE] Warning! Stepping down from term %v.\n", raft.currentTerm)
				log.Printf("[CANDIDATE] %v is running in a new term %v .\n", rvr.peerIndex, rvr.Term)
				return

            } else {

                log.Printf("[CANDIDATE] This vote has denied by peer %v for term %v.\n", rvr.peerIndex, raft.currentTerm)
            }

            break

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		case rv := <-raft.requestVoteChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

            if rv.Term > raft.currentTerm {

                raft.updateStates(updatedVotedFor, updatedTerm, follower)
				raft.requestVoteChan <- rv
                log.Printf("[CANDIDATE] Warning! Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[CANDIDATE] %v is running for a new term %v.\n", rv.CandidateID, rv.Term)
                log.Printf("[CANDIDATE] Vote to peer %v for term %v.\n", rv.CandidateID, rv.Term)
				return

            } else {

                log.Printf("[CANDIDATE] Vote denied to peer %v for term %v.\n", rv.CandidateID, rv.Term)

	            replyVote := &RequestVoteReply{
	                VoteGranted: false,
					Term: raft.currentTerm,
				}

				rv.replyChan <- replyVote
				break
            }

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		case ae := <-raft.appendEntryChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

            if ae.Term == raft.currentTerm {

				raft.updateStates(updatedVotedFor, updatedTerm, follower)
                raft.appendEntryChan <- ae
				log.Printf("[CANDIDATE] %v was elected for term %v.\n", ae.LeaderID, ae.Term)
                log.Printf("[CANDIDATE] Warning! Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[CANDIDATE] AppendEntry was accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)
				return

            } else if ae.Term > raft.currentTerm {

				raft.updateStates(nullVote, ae.Term, follower)
                raft.appendEntryChan <- ae
				log.Printf("[CANDIDATE] %v is leading a new term %v.\n", ae.LeaderID, ae.Term)
                log.Printf("[CANDIDATE] Warning! Stepping down from term %v.\n", raft.currentTerm)
                log.Printf("[CANDIDATE] AppendEntry accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)
				return

            } else {

                log.Printf("[CANDIDATE] AppendEntry was rejected from peer %v for term %v.\n", ae.LeaderID, ae.Term)
	            replySuccess := &AppendEntryReply{
	                Success: false,
					Term: raft.currentTerm,
				}
				ae.replyChan <- replySuccess
				break

            }

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		}
	}
}

// leaderSelect implements the logic to handle messages from distinct
// events when in leader state.
func (raft *Raft) leaderSelect() {
	log.Println("[LEADER] Run Logic.")

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

    log.Printf("[LEADER] Assuming term %v.\n", raft.currentTerm)

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

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

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

		case aer := <-replyChan:

			if aer.Term > raft.currentTerm {

				raft.updateStates(nullVote, aer.Term, follower)
				log.Printf("[LEADER] %v is running in a new term %v.\n", aer.peerIndex, aer.Term)
				log.Printf("[LEADER] Stepping down from term %v.\n", raft.currentTerm)
				return

			}

			break

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		case rv := <-raft.requestVoteChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

			vote := false

			if rv.Term > raft.currentTerm {

				vote = true
				raft.updateStates(rv.CandidateID, rv.Term, follower)
				log.Printf("[LEADER] %v is running for a new term %v.\n", rv.CandidateID, rv.Term)
				log.Printf("[LEADER] Warning! Stepping down from term %v.\n", raft.currentTerm)
				log.Printf("[LEADER] Vote granted to peer %v for term %v.\n", rv.CandidateID, rv.Term)

			} else {

				log.Printf("[LEADER] Vote denied to peer %v for term %v.\n", rv.CandidateID, rv.Term)

			}

			replyVote := &RequestVoteReply {
				Term: raft.currentTerm,
				VoteGranted: vote,
			}

			rv.replyChan <- replyVote

			if vote {

				return

			} else {

				break

			}

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////

		case ae := <-raft.appendEntryChan:

///////////////////////////////////
// EU MODIFIQUEI O CÓDIGO AQUI!  //
///////////////////////////////////

			ok := false

			if ae.Term == raft.currentTerm {

				ok = true
				log.Printf("[LEADER] AppendEntry denied from peer %v for term %v.\n", ae.LeaderID, ae.Term)
				panic(errors.New("[LEADER] Error: multiple leaders for current term.\n"))

			} else if ae.Term > raft.currentTerm {

				ok = true
				raft.updateStates(nullVote, ae.Term, follower)
				raft.appendEntryChan <- ae
				log.Printf("[LEADER] %v is leading in a new term %v.\n", ae.LeaderID, ae.Term)
				log.Printf("[LEADER] Stepping down from term %v.\n", raft.currentTerm)
				log.Printf("[LEADER] AppendEntry accepted from peer %v for term %v.\n", ae.LeaderID, ae.Term)

			} else {

				log.Printf("[LEADER] AppendEntry denied from peer %v for term %v.\n", ae.LeaderID, ae.Term)

			}

			replyVote := &AppendEntryReply{
				Term: raft.currentTerm,
				Success: ok,
			}

			ae.replyChan <- replyVote

			if ok {

				return

			} else {

				break

			}

//////////////////////////////
// FIM DA MINHA MODIFICAÇÃO //
//////////////////////////////
		}
	}
}
