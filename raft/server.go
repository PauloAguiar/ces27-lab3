package raft

import (
	"log"
	"net"
	"net/rpc"
)

type server struct {
	hostname string
	net.Listener
	rpcs *RPC
}

func newServer(raft *Raft, hostname string) (*server, error) {
	var serv *server

	serv = &server{
		rpcs:     &RPC{raft},
		hostname: hostname,
	}

	err := rpc.Register(serv.rpcs)

	if err != nil {
		return nil, err
	}

	return serv, nil
}

func (s *server) startListening() error {
	var err error

	s.Listener, err = net.Listen("tcp", s.hostname)

	if err != nil {
		return err
	}

	log.Printf("[SERVER] Listening on '%v'\n", s.hostname)

	go s.acceptConnections()

	return nil
}

func (s *server) acceptConnections() {
	var (
		err  error
		conn net.Conn
	)

	log.Printf("[SERVER] Accepting connections.\n")

	for {
		conn, err = s.Accept()

		if err != nil {
			break
		}

		go rpc.ServeConn(conn)
	}

	log.Printf("[SERVER] Ended accepting connections. Err: %v\n", err)
}
