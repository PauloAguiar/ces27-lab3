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
	log.Println("[FOLLOWER] Run Logic.")
	raft.resetElectionTimeout()
	for {
		select {
		case <-raft.electionTick:
			// Na falta de um líder, se candidata
			log.Println("[FOLLOWER] Election timeout.")
			raft.currentState.Set(candidate)
			return

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
			// Atualiza turno de eleição
			if rv.Term > raft.currentTerm {
				raft.currentTerm = rv.Term
				raft.votedFor = 0
			}
			// Se ainda não votou nessa eleição
			if rv.Term == raft.currentTerm &&
				(raft.votedFor == 0 || raft.votedFor == rv.CandidateID) {
				// Realiza o seu voto
				raft.votedFor = rv.CandidateID
				// Se votou, não precisa se candidatar
				log.Printf("[FOLLOWER] Vote granted to '%v' for term '%v'.\n",
					raft.peers[rv.CandidateID], raft.currentTerm)
				raft.resetElectionTimeout()
				// Vota no candidato que solicitou voto
				reply := &RequestVoteReply{
					Term:        raft.currentTerm,
					VoteGranted: true,
				}
				rv.replyChan <- reply
				break
			}

			// Se já votou na eleição, rejeita candidato
			log.Printf("[FOLLOWER] Vote denied to '%v' for term '%v'.\n",
				raft.peers[rv.CandidateID], raft.currentTerm)
			reply := &RequestVoteReply{
				Term:        raft.currentTerm,
				VoteGranted: false,
			}
			rv.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			// Ignora líder velho - que logo morrerá
			if ae.Term < raft.currentTerm {
				break
			}
			// Atualiza o mandato - descoberta de novo líder
			if ae.Term > raft.currentTerm {
				raft.currentTerm = ae.Term
				raft.votedFor = 0
			}

			// Heartbeat para o líder atual
			log.Printf("[FOLLOWER] Accept AppendEntry from '%v'.\n",
				raft.peers[ae.LeaderID])
			raft.resetElectionTimeout()
			reply := &AppendEntryReply{
				Term:    raft.currentTerm,
				Success: true,
			}
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
		case rvr := <-replyChan:
			///////////////////
			//  MODIFY HERE  //

			// Verifica se recebeu o voto
			if rvr.VoteGranted {
				voteCount++
				log.Printf("[CANDIDATE] Vote granted by '%v'.\n",
					raft.peers[rvr.peerIndex])
				log.Println("[CANDIDATE] VoteCount: ", voteCount)

				// Se a maioria votou nele, se torna líder
				if voteCount > len(raft.peers)/2 {
					log.Println("[CANDIDATE] is now Elected")
					raft.currentState.Set(leader)
					return
				}
			} else {
				log.Printf("[CANDIDATE] Vote denied by '%v'.\n",
					raft.peers[rvr.peerIndex])
			}
			break
			// END OF MODIFY //
			///////////////////

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
			// Se descobrir nova eleição, volta a ser seguidor
			if rv.Term > raft.currentTerm {
				log.Printf("[CANDIDATE] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.requestVoteChan <- rv
				return
			}
			// É candidato ou eleição é velha, então não vota
			log.Printf("[CANDIDATE] Vote denied to '%v' for term '%v'.\n",
				raft.peers[rv.CandidateID], raft.currentTerm)
			reply := &RequestVoteReply{
				Term:        raft.currentTerm,
				VoteGranted: false,
			}
			rv.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			// Se descobrir novo líder, volta a ser seguidor
			if ae.Term >= raft.currentTerm {
				log.Printf("[CANDIDATE] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.appendEntryChan <- ae
				return
			}
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
			// Resposta positiva do heartbeat
			_ = aet
			break
			// END OF MODIFY //
			///////////////////
		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
			// Nova eleição mata líder antigo
			if rv.Term > raft.currentTerm {
				log.Printf("[LEADER] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.requestVoteChan <- rv
				return
			}
			// Se a eleição é velha, rejeita candidatos
			log.Printf("[LEADER] Vote denied to '%v' for term '%v'.\n",
				raft.peers[rv.CandidateID], raft.currentTerm)
			reply := &RequestVoteReply{
				Term:        raft.currentTerm,
				VoteGranted: false,
			}
			rv.replyChan <- reply
			break

			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			// Se existe um líder mais novo, o velho morre
			if ae.Term > raft.currentTerm {
				log.Printf("[LEADER] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.appendEntryChan <- ae
				return
			}
			// Se o líder for velho, rejeita-o
			log.Printf("[LEADER] Rejected AppendEntry from '%v'.\n",
				raft.peers[ae.LeaderID])
			reply := &AppendEntryReply{
				Term:    raft.currentTerm,
				Success: false,
			}
			ae.replyChan <- reply
			break
			// END OF MODIFY //
			///////////////////
		}
	}
}
