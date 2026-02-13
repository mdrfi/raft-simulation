package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"hashicorp-raft-demo/raftnode"

	"github.com/hashicorp/raft"
)

func main() {
	id := envInt("NODE_ID", 0)
	port := envStr("PORT", "8080")
	raftPort := envStr("RAFT_PORT", "7000")
	dataDir := envStr("DATA_DIR", "/data")
	peersStr := envStr("PEERS", "")

	allPeers := parsePeers(peersStr)
	var myPeers []raftnode.Peer
	for _, p := range allPeers {
		if p.ID != id {
			myPeers = append(myPeers, p)
		}
	}

	log.Printf("[node-%d] starting with %d peers, data=%s, http=:%s, raft=:%s",
		id, len(myPeers), dataDir, port, raftPort)
	for _, p := range myPeers {
		log.Printf("[node-%d]   peer %d @ %s", id, p.ID, p.Addr)
	}

	hostname, _ := os.Hostname()
	bindAddr := net.JoinHostPort("0.0.0.0", raftPort)
	advertiseAddr := net.JoinHostPort(hostname, raftPort)

	svcName := fmt.Sprintf("node-%d", id)
	advertiseAddr = net.JoinHostPort(svcName, raftPort)

	log.Printf("[node-%d] raft bind=%s advertise=%s", id, bindAddr, advertiseAddr)

	node, err := raftnode.NewNode(raftnode.NodeConfig{
		ID:            id,
		BindAddr:      bindAddr,
		AdvertiseAddr: advertiseAddr,
		DataDir:       dataDir,
		Peers:         myPeers,
	})
	if err != nil {
		log.Fatalf("[node-%d] failed to create node: %v", id, err)
	}

	var servers []raft.Server
	for _, p := range allPeers {
		pHost, _, _ := net.SplitHostPort(p.Addr)
		servers = append(servers, raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("%d", p.ID)),
			Address: raft.ServerAddress(net.JoinHostPort(pHost, raftPort)),
		})
	}

	found := false
	for _, s := range servers {
		if string(s.ID) == fmt.Sprintf("%d", id) {
			found = true
			break
		}
	}
	if !found {
		servers = append(servers, raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("%d", id)),
			Address: raft.ServerAddress(advertiseAddr),
		})
	} else {

		for i, s := range servers {
			if string(s.ID) == fmt.Sprintf("%d", id) {
				servers[i].Address = raft.ServerAddress(advertiseAddr)
				break
			}
		}
	}

	node.Bootstrap(servers)

	mux := raftnode.NewHTTPServer(node)
	addr := ":" + port
	log.Printf("[node-%d] HTTP listening on %s", id, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[node-%d] HTTP server error: %v", id, err)
	}
}

func parsePeers(s string) []raftnode.Peer {
	var peers []raftnode.Peer
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		idStr := part[:eqIdx]
		addr := part[eqIdx+1:]
		pid, err := strconv.Atoi(idStr)
		if err != nil {
			log.Printf("warning: cannot parse peer id %q", idStr)
			continue
		}
		peers = append(peers, raftnode.Peer{ID: pid, Addr: addr})
	}
	return peers
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid %s=%q, using %d\n", key, v, fallback)
		return fallback
	}
	return n
}
