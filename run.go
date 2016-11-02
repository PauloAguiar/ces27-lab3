package main

import (
	"flag"
	"hash/fnv"
	"log"
	"math/rand"
	"time"
	"github.com/pauloaguiar/ces27-lab3/raft"
)

// Command line parameters
var (
	instanceID = flag.Int("id", 1, "ID of the instance")
	seed       = flag.String("seed", "", "Seed for RNG")
)

func main() {
	flag.Parse()

	if *seed != "" {
		h := fnv.New32a()
		h.Write([]byte(*seed))
		rand.Seed(int64(h.Sum32()))
	} else {
		rand.Seed(time.Now().UnixNano())
	}

	peers := make(map[int]string)
	peers[1] = "localhost:3001"
	peers[2] = "localhost:3002"
	peers[3] = "localhost:3003"
	peers[4] = "localhost:3004"
	peers[5] = "localhost:3005"

	if _, ok := peers[*instanceID]; !ok {
		log.Fatalf("[MAIN] Invalid instance id.\n")
	}

	raft := raft.NewRaft(peers, *instanceID)

	<-raft.Done()
}
