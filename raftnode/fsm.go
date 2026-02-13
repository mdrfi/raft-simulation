package raftnode

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

type FSM struct {
	mu       sync.Mutex
	entries  []LogEntry
	recorder *EventRecorder
	nodeID   int
}

func NewFSM(nodeID int, recorder *EventRecorder) *FSM {
	return &FSM{
		recorder: recorder,
		nodeID:   nodeID,
	}
}

func (f *FSM) Apply(l *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	data := string(l.Data)
	entry := LogEntry{
		Term:  l.Term,
		Index: l.Index,
		Data:  data,
	}
	f.entries = append(f.entries, entry)

	f.recorder.Record(TrackedEvent{
		Type:       "commit",
		From:       f.nodeID,
		To:         -1,
		Term:       int(l.Term),
		EntryIndex: int(l.Index),
		Extra:      map[string]interface{}{"data": data},
	})

	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]LogEntry, len(f.entries))
	copy(cp, f.entries)
	return &fsmSnapshot{entries: cp}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	var entries []LogEntry
	if err := json.NewDecoder(rc).Decode(&entries); err != nil {
		return err
	}
	f.entries = entries
	return nil
}

func (f *FSM) Entries() []LogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]LogEntry, len(f.entries))
	copy(cp, f.entries)
	return cp
}

func (f *FSM) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

type fsmSnapshot struct {
	entries []LogEntry
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s.entries)
	if err != nil {
		sink.Cancel()
		return err
	}
	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}
