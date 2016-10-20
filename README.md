# CES-27 - LAB 3 - Raft Consensus Algorithm

Instruções para configuração do ambiente: [Setup](SETUP.md)  
Para saber mais sobre a utilização do Git, acessar: [Git Passo-a-Passo](GIT.md)  
Para saber mais sobre a entrega do trabalho, acessar: [Entrega](ENTREGA.md)  

## Referências

(Paper) [In Search of an Understandable Consensus Algorithm(Extended Version)](https://raft.github.io/raft.pdf)  
(Presentation) [Raft: A Consensus Algorithm for Replicated Logs](https://raft.github.io/slides/raftuserstudy2013.pdf)  
(Animation) [The Secret Lives of Data: Raft - Understandable Distributed Consensus](http://thesecretlivesofdata.com/raft/)  
(Simulador) [Raft Visualization](https://raft.github.io/)  


# Executando o Código

Durante a execução dessa atividade, será necessário que sejam inicializadas cinco instâncias do serviço proposto. O arquivo main da execução(*run.go*) se encontra na pasta raiz do projeto.

A primeira instância pode ser inicializada como mostrado a seguir:
```bash
$ go run run.go -id 1
```
> [SERVER] Listening on 'localhost:3001'  
> [FOLLOWER] Run Logic.  
> [SERVER] Accepting connections.  

As demais instâncias devem utilizar o seguinte comando (incrementando o parâmetro id para cade instância):
```bash
$ go run run.go -id #
```

O Resultado da execução é mostrado a seguir:
![Run All](doc/run-all.PNG?raw=true)

Para terminar as instâncias, utilizar a combinação de teclas Ctrl+C


# Implementação

O código fornecido não implementa o protocolo de eleição de lider proposto no paper. Por conta disso, as instâncias nunca chegarão a uma decisão.

As instâncias podem estar em três estados:

* Follower
* Candidate 
* Leader 

A comunicação entre as instâncias é feita através de duas operações:

* RequestVote

Enviada pelos candidatos para verificar se eles possuem a maioria dos votos.
RequestVoteArgs | |
--- |:---:
Term  | candidate's term
CandidateID | candidate requesting vote


RequestVoteReply | |
--- |:---:|
Term | currentTerm, for candidate to update itself
VoteGranted | rue means candidate received vote           |

 * AppendEntry  
  
Enviada pelo líder para as outras instâncias a fim de transmitir informação e servir como Heart beat.

AppendEntryArgs | |
--- |:---:
Term | leader’s term
LeaderID | so follower can redirect clients


AppendEntryReply | |
--- |:---:
Term | currentTerm, for leader to update itself
Success | true if operation matched requirements

A simulação [Leader Election](http://thesecretlivesofdata.com/raft/#election) explica de forma bastante clara a relação entre os estados e as operações para que a eleição aconteça.

Para que a eleição do líder ocorra de forma correta, 8 trechos de código devem ser implementadas:

No arquivo *raft/raft.go*:

Para o estado **follower**, devem ser implementados os trechos que processam e geram as respostas para as duas operações descritas acima:
 
```go
func (raft *Raft) followerSelect() {
	log.Println("[FOLLOWER] Run Logic.")
	raft.resetElectionTimeout()
	for {
		(...)
		case rv := <-raft.requestVoteChan:
			// Request Vote Operation
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			// Append Entry Operation 
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////
		}
	}
}
```

Para o estado **candidate**, devem ser implementados os trechos que processam e geram as respostas para as duas operações descritas acima, além de um trecho que gerencia as respostas dos envios de operações RequestVote para outras instâncias:
 
```go
func (raft *Raft) candidateSelect() {
	(...)
	for {
		select {
		(...)
		case rvr := <-replyChan:
			// Request Vote Responses
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////

		case rv := <-raft.requestVoteChan:
			// Request Vote Operation
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			// Append Entry Operation 
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////
		}
	}
}
```
Para o estado **leader**, devem ser implementados os trechos que processam e geram as respostas para as duas operações descritas acima, além de um trecho que gerencia as respostas dos envios de operações AppendEntry para outras instâncias:
 
```go
func (raft *Raft) leaderSelect() {
	(...)
	for {
		select {
			(...)
		case aet := <-replyChan:
			// Append Entry Responses
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////

		case rv := <-raft.requestVoteChan:
			// Request Vote Operation
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////

		case ae := <-raft.appendEntryChan:
			// Append Entry Operation 
			///////////////////
			//  MODIFY HERE  //
			(...)
			// END OF MODIFY //
			///////////////////
		}
	}
}
```

A lógica para a implementação deve ser a descrita no seguinte sumário:

![Protocol Summary](doc/protocol-summary.PNG?raw=true)

## Informações importantes:

* Ao alterar o currentTerm em uma instância, você deve sempre alterar o votedFor, normalmente deixando-o em 0 (ou seja, não votou naquele term)

* Atenção ao utilizar comandos de quebra do fluxo:

```go
	// Utilizar return quando houver uma alteração no estado para 
	// retornar à função loop.
	raft.currentState.Set(leader)
	return

	// Utilizar break para sair do select (switch para canais), 
	// ou seja, no fim de uma operação, para continuar no processamento
	// do estado atual.
	reply.Success = false
	ae.replyChan <- reply
	break
```

* Algumas subrotinas não devem ser chamadas. Não alterar subrotinas fora do escopo pedido. Incluíndo:
	* loop
	* AppendEntry
	* broadcastAppendEntries
	* sendAppendEntry
	* RequestVote
	* broadcastRequestVote
	* sendRequestVote
	* CallHost
	* startListening
	* acceptConnections
	* electionTimeout
	* broadcastInterval

* Tenha certeza de resetar o electionTimer exatamente como descrito no sumário. Especificamente, você deve resetar o electionTimer quando:
	
	a) Ao receber um AppendEntry do líder atual, ou seja, quando este possuir um Term válido;  
	b) Ao iniciar um eleição;  
	c) Ao votar em um candidado.

* Numa operação de *Step Down*, você deve fazer a alteração no estado imediatamente e processar novamente no novo estado, para isso basta recolocar o objeto da chamada no canal original:

```go
	// 3. (step down if leader or candidate)
	log.Printf("[LEADER] Stepping down.\n")
	raft.currentState.Set(follower)
	raft.appendEntryChan <- ae
	return
```
# Verificação

Quando a implementação estiver correta, as instância sempre vão decidir por um líder (podem ocorrer várias votações antes de chegarem a um consenso).

A imagem abaixo mostra a situação estável:

![Happy Path](doc/run-all-happy.PNG?raw=true)

Para introduzir uma falha, basta encerrar um dos processos. Neste caso estamos interessado numa falha do líder, para que ocorra uma nova eleição:

A imagem abaixo mostra um caso em que, após a falha do líder, o nó 5 é eleito para o *Term* 4, ou seja após 3 votações.
![Leader Failure](doc/run-all-failure.PNG?raw=true)

Quando a instância que falhou retorna, ela é atualizada (através da chamada AppendEntry)
![Leader Failure](doc/run-all-recovery.PNG?raw=true)

Experimente gerar várias falhas (3 ou mais vão deixar o sistema incapaz de decidir por um novo líder).