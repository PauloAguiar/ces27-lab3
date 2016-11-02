package raft

import "net/rpc"

// RPC is the struct that implements the methods exposed to RPC calls
type RPC struct {
	raft *Raft
}

// CallHost will communicate to another host through it's RPC public API.
func (raft *Raft) CallHost(index int, method string, args interface{}, reply interface{}) error {
	var (
		err    error
		client *rpc.Client
	)

	client, err = rpc.Dial("tcp", raft.peers[index])
	if err != nil {
		return err
	}

	defer client.Close()

	err = client.Call("RPC."+method, args, reply)

	if err != nil {
		return err
	}

	return nil
}
