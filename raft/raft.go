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
			log.Println("[FOLLOWER] Election timeout.")
			raft.currentState.Set(candidate)
			return

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //

			reply := &RequestVoteReply{
				Term: raft.currentTerm,
			}

			if rv.Term > raft.currentTerm {
				//caso em que é uma nova eleição
				raft.currentTerm = rv.Term
				raft.votedFor = rv.CandidateID
				reply.VoteGranted = true
				log.Printf("[FOLLOWER] Vote granted to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
				raft.resetElectionTimeout()
			} else if rv.Term == raft.currentTerm && raft.votedFor == 0 {
								//caso em que e uma eleicao em andamento mas ainda nao houve voto
								raft.votedFor = rv.CandidateID
								reply.VoteGranted = true
								log.Printf("[FOLLOWER] Vote granted to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
								raft.resetElectionTimeout()
							} else {
									//caso em que ja tenha votado nessa eleicao ou que o pedido seja para uma eleicao antiga
									reply.VoteGranted = false
									log.Printf("[FOLLOWER] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
								}

			rv.replyChan <- reply
			break
			// END OF MODIFY - FEITO!!!!//
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			reply := &AppendEntryReply{
				Term: raft.currentTerm,
			}

			if ae.Term >= raft.currentTerm {
				log.Printf("[FOLLOWER] Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
				reply.Success = true
				raft.resetElectionTimeout()
			} else {
					log.Printf("[FOLLOWER] Do not Accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
					reply.Success =false
					// Nao reseta o ElectionTimeout
			}

			ae.replyChan <- reply
			break
			// END OF MODIFY // FEITO !!!!
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

			if rvr.VoteGranted {
				log.Printf("[CANDIDATE] Vote granted by '%v'.\n", raft.peers[rvr.peerIndex])
				voteCount++
				log.Println("[CANDIDATE] VoteCount: ", voteCount)

				//verifica se ja teve maioria na votacao e se torna leader
				if voteCount > (len(raft.peers)/2) {
					log.Println("[CANDIDATE elected as Leader]")
					raft.currentState.Set(leader)
					return //abandona a funcao para executar como leader
					}

			} else { //Vote not granted
				log.Printf("[CANDIDATE] Vote denied by '%v'.\n", raft.peers[rvr.peerIndex])
			}
			break
			// END OF MODIFY // FEITO!!!!
			///////////////////

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //

			if rv.Term > raft.currentTerm {
				//faz step Down e se re-envia a mensagem recebida
				log.Printf("[CANDIDATE] Stepping down.\n")
    		raft.currentState.Set(follower)
    		raft.requestVoteChan <- rv
				return
			} else { //nesse caso o requestVote vai ser negado
				log.Printf("[CANDIDATE] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
				reply := &RequestVoteReply{
					Term: raft.currentTerm,
				}
				reply.VoteGranted = false
				rv.replyChan <- reply
			}

			break
			// END OF MODIFY // FEITO !!!
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //


			if ae.Term >= raft.currentTerm {
				//faz step Down e se re-envia a mensagem recebida
				log.Printf("[CANDIDATE] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.appendEntryChan <- ae
				return
			} else{
					reply := &AppendEntryReply{
						Term: raft.currentTerm,
					}

					log.Printf("[CANDIDATE] Do not accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
					reply.Success = false
					ae.replyChan <- reply
			}
			break
			// END OF MODIFY // FEITO !!!
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
			// Respostas recebidas ao comando appendEntry
			///////////////////
			//  MODIFY HERE  //
			if aet.Term > raft.currentTerm {
				//se alguma resposta tem Term maior do que o currentTerm faz step Down
				log.Printf("[LEADER] Stepping down.\n")
				raft.currentState.Set(follower)
				return
			}
			//aqui deve ser tratado o commit caso haja maioria de respostas bem sucedidas
			
			// END OF MODIFY // FEITO!!!
			///////////////////

		case rv := <-raft.requestVoteChan:
			///////////////////
			//  MODIFY HERE  //
			if rv.Term > raft.currentTerm {
				//faz step Down e se re-envia a mensagem recebida
				log.Printf("[LEADER] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.requestVoteChan <- rv
				return
			} else { //nesse caso o requestVote vai ser negado
				log.Printf("[LEADER] Vote denied to '%v' for term '%v'.\n", raft.peers[rv.CandidateID], raft.currentTerm)
				reply := &RequestVoteReply{
					Term: raft.currentTerm,
				}
				reply.VoteGranted = false
				rv.replyChan <- reply
			}
			break
			// END OF MODIFY // FEITO !!!!
			///////////////////

		case ae := <-raft.appendEntryChan:
			///////////////////
			//  MODIFY HERE  //
			if ae.Term > raft.currentTerm {
				//faz step Down e se re-envia a mensagem recebida
				log.Printf("[LEADER] Stepping down.\n")
				raft.currentState.Set(follower)
				raft.appendEntryChan <- ae
				return
			} else {
				reply := &AppendEntryReply{
					Term: raft.currentTerm,
				}

				log.Printf("[LEADER] Do not accept AppendEntry from '%v'.\n", raft.peers[ae.LeaderID])
				reply.Success = false
				ae.replyChan <- reply

			}
			break
			// END OF MODIFY // FEITO !!!!
			///////////////////
		}
	}
}
