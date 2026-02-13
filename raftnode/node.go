package raftnode

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
)

type Node struct {
	mu sync.Mutex

	ID      int
	dataDir string

	raft      *raft.Raft
	fsm       *FSM
	transport *raft.NetworkTransport
	logStore  raft.LogStore
	stable    raft.StableStore
	snaps     raft.SnapshotStore
	recorder  *EventRecorder

	paused  bool
	crashed bool

	peers []Peer

	bindAddr      string
	advertiseAddr string

	obsCh chan raft.Observation

	lastState string
}

type NodeConfig struct {
	ID            int
	BindAddr      string
	AdvertiseAddr string
	DataDir       string
	Peers         []Peer
}

func NewNode(cfg NodeConfig) (*Node, error) {
	recorder := NewEventRecorder()

	fsm := NewFSM(cfg.ID, recorder)

	rc := raft.DefaultConfig()
	rc.LocalID = raft.ServerID(fmt.Sprintf("%d", cfg.ID))
	rc.HeartbeatTimeout = 1000 * time.Millisecond
	rc.ElectionTimeout = 1500 * time.Millisecond
	rc.LeaderLeaseTimeout = 500 * time.Millisecond
	rc.CommitTimeout = 200 * time.Millisecond
	rc.SnapshotInterval = 120 * time.Second
	rc.SnapshotThreshold = 8192

	rc.LogLevel = "WARN"

	os.MkdirAll(cfg.DataDir, 0o755)

	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	snaps, err := raft.NewFileSnapshotStore(cfg.DataDir, 2, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}

	var advertise net.Addr
	if cfg.AdvertiseAddr != "" {
		adv, err2 := net.ResolveTCPAddr("tcp", cfg.AdvertiseAddr)
		if err2 != nil {
			return nil, fmt.Errorf("resolve advertise addr: %w", err2)
		}
		advertise = adv
	}
	transport, err := raft.NewTCPTransport(cfg.BindAddr, advertise, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	r, err := raft.NewRaft(rc, fsm, logStore, stableStore, snaps, transport)
	if err != nil {
		return nil, fmt.Errorf("new raft: %w", err)
	}

	n := &Node{
		ID:            cfg.ID,
		dataDir:       cfg.DataDir,
		raft:          r,
		fsm:           fsm,
		transport:     transport,
		logStore:      logStore,
		stable:        stableStore,
		snaps:         snaps,
		recorder:      recorder,
		peers:         cfg.Peers,
		bindAddr:      cfg.BindAddr,
		advertiseAddr: cfg.AdvertiseAddr,
		lastState:     "follower",
	}

	recorder.Record(TrackedEvent{
		Type: "state_change", From: cfg.ID, To: -1, Term: 0, NodeState: "follower",
	})

	n.obsCh = make(chan raft.Observation, 64)
	r.RegisterObserver(raft.NewObserver(n.obsCh, true, func(o *raft.Observation) bool {
		_, isLeader := o.Data.(raft.LeaderObservation)
		_, isRaftState := o.Data.(raft.RaftState)
		return isLeader || isRaftState
	}))
	go n.observeLoop()

	return n, nil
}

func (n *Node) Bootstrap(allServers []raft.Server) {
	cfg := raft.Configuration{Servers: allServers}
	f := n.raft.BootstrapCluster(cfg)
	if err := f.Error(); err != nil {

		log.Printf("[node-%d] bootstrap: %v", n.ID, err)
	}
}

func (n *Node) observeLoop() {
	for o := range n.obsCh {
		n.mu.Lock()
		switch v := o.Data.(type) {
		case raft.LeaderObservation:
			leaderAddr := v.LeaderAddr
			if leaderAddr == n.transport.LocalAddr() {
				n.recorder.Record(TrackedEvent{
					Type:      "leader_elected",
					From:      n.ID,
					To:        -1,
					Term:      parseInt(n.raft.Stats()["term"]),
					NodeState: "leader",
				})
			}
		case raft.RaftState:
			state := v.String()
			if state != n.lastState {
				n.lastState = state
				n.recorder.Record(TrackedEvent{
					Type:      "state_change",
					From:      n.ID,
					To:        -1,
					NodeState: state,
				})
			}
		}
		n.mu.Unlock()
	}
}

func (n *Node) Status() NodeStatus {
	n.mu.Lock()
	paused := n.paused
	crashed := n.crashed
	n.mu.Unlock()

	stats := n.raft.Stats()

	state := n.raft.State().String()
	if crashed {
		state = "crashed"
	} else if paused {
		state = "paused"
	}

	term := parseInt(stats["term"])
	commitIdx := parseInt(stats["commit_index"])
	lastLogIdx := parseInt(stats["last_log_index"])

	return NodeStatus{
		ID:          n.ID,
		State:       state,
		Term:        term,
		CommitIndex: commitIdx,
		LogLength:   lastLogIdx + 1,
		Paused:      paused,
		Crashed:     crashed,
	}
}

func (n *Node) Events() []TrackedEvent {
	return n.recorder.Events()
}

func (n *Node) GetLog() []LogEntry {
	return n.fsm.Entries()
}

func (n *Node) HandleClientWrite(data string) ClientWriteResponse {
	n.mu.Lock()
	paused := n.paused
	crashed := n.crashed
	n.mu.Unlock()

	if crashed || paused {
		return ClientWriteResponse{OK: false, Error: "node not available"}
	}

	if n.raft.State() != raft.Leader {
		return ClientWriteResponse{OK: false, Error: "not leader"}
	}

	n.recorder.Record(TrackedEvent{
		Type:  "client_write",
		From:  n.ID,
		To:    -1,
		Term:  int(n.currentTerm()),
		Extra: map[string]interface{}{"data": data},
	})

	f := n.raft.Apply([]byte(data), 5*time.Second)
	if err := f.Error(); err != nil {
		return ClientWriteResponse{OK: false, Error: err.Error()}
	}
	return ClientWriteResponse{OK: true}
}

func (n *Node) Pause(dur time.Duration) {
	n.mu.Lock()
	n.paused = true
	n.recorder.Record(TrackedEvent{
		Type: "node_paused", From: n.ID, To: -1,
		Extra: map[string]interface{}{"duration_ms": dur.Milliseconds()},
	})
	n.mu.Unlock()

	time.AfterFunc(dur, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.paused {
			n.paused = false
			n.recorder.Record(TrackedEvent{Type: "node_resumed", From: n.ID, To: -1})
		}
	})
}

func (n *Node) Crash() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.crashed {
		return
	}
	n.crashed = true
	n.recorder.Record(TrackedEvent{Type: "node_crashed", From: n.ID, To: -1})

	f := n.raft.Shutdown()
	if err := f.Error(); err != nil {
		log.Printf("[node-%d] shutdown error: %v", n.ID, err)
	}
}

func (n *Node) Restart() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.crashed = false
	n.paused = false

	rc := raft.DefaultConfig()
	rc.LocalID = raft.ServerID(fmt.Sprintf("%d", n.ID))
	rc.HeartbeatTimeout = 1000 * time.Millisecond
	rc.ElectionTimeout = 1500 * time.Millisecond
	rc.LeaderLeaseTimeout = 500 * time.Millisecond
	rc.CommitTimeout = 200 * time.Millisecond
	rc.SnapshotInterval = 120 * time.Second
	rc.SnapshotThreshold = 8192
	rc.LogLevel = "WARN"

	var advertise net.Addr
	if n.advertiseAddr != "" {
		adv, resolveErr := net.ResolveTCPAddr("tcp", n.advertiseAddr)
		if resolveErr != nil {
			log.Printf("[node-%d] restart resolve advertise error: %v", n.ID, resolveErr)
			return
		}
		advertise = adv
	}

	snaps, err := raft.NewFileSnapshotStore(n.dataDir, 2, os.Stderr)
	if err != nil {
		log.Printf("[node-%d] restart snapshot store error: %v", n.ID, err)
		return
	}

	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	transport, err := raft.NewTCPTransport(n.bindAddr, advertise, 3, 10*time.Second, os.Stderr)
	if err != nil {
		log.Printf("[node-%d] restart transport error: %v", n.ID, err)
		return
	}

	r, err := raft.NewRaft(rc, n.fsm, logStore, stableStore, snaps, transport)
	if err != nil {
		log.Printf("[node-%d] restart raft error: %v", n.ID, err)
		return
	}

	n.raft = r
	n.transport = transport
	n.logStore = logStore
	n.stable = stableStore
	n.snaps = snaps
	n.lastState = "follower"

	selfAddr := n.advertiseAddr
	if selfAddr == "" {
		selfAddr = n.bindAddr
	}
	servers := []raft.Server{
		{
			ID:      raft.ServerID(fmt.Sprintf("%d", n.ID)),
			Address: raft.ServerAddress(selfAddr),
		},
	}
	for _, p := range n.peers {
		servers = append(servers, raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("%d", p.ID)),
			Address: raft.ServerAddress(peerRaftAddr(p.Addr)),
		})
	}
	r.BootstrapCluster(raft.Configuration{Servers: servers})

	n.obsCh = make(chan raft.Observation, 64)
	r.RegisterObserver(raft.NewObserver(n.obsCh, true, func(o *raft.Observation) bool {
		_, isLeader := o.Data.(raft.LeaderObservation)
		_, isRaftState := o.Data.(raft.RaftState)
		return isLeader || isRaftState
	}))
	go n.observeLoop()

	n.recorder.Record(TrackedEvent{
		Type:      "node_restarted",
		From:      n.ID,
		To:        -1,
		Term:      int(n.currentTerm()),
		NodeState: "follower",
	})
}

func (n *Node) PersistEvents() error {
	path := filepath.Join(n.dataDir, fmt.Sprintf("node-%d-events.jsonl", n.ID))
	return n.recorder.PersistEvents(path)
}

func (n *Node) currentTerm() uint64 {
	stats := n.raft.Stats()
	return parseUint64(stats["term"])
}

func peerRaftAddr(httpAddr string) string {

	host, _, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return httpAddr
	}
	return net.JoinHostPort(host, "7000")
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func parseUint64(s string) uint64 {
	var n uint64
	fmt.Sscanf(s, "%d", &n)
	return n
}
