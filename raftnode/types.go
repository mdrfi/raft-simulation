package raftnode

import (
	"encoding/json"
	"time"
)

type LogEntry struct {
	Term  uint64 `json:"term"`
	Index uint64 `json:"index"`
	Data  string `json:"data"`
}

type TrackedEvent struct {
	SimTimeMs  int64                  `json:"sim_time_ms"`
	Type       string                 `json:"type"`
	Term       int                    `json:"term"`
	From       int                    `json:"from"`
	To         int                    `json:"to"`
	EntryIndex int                    `json:"entry_index,omitempty"`
	NodeState  string                 `json:"node_state,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

type Summary struct {
	SimDurationMs    int64       `json:"sim_duration_ms"`
	LeaderChanges    int         `json:"leader_changes"`
	TotalCommits     int         `json:"total_commits"`
	SafetyViolation  bool        `json:"safety_violation"`
	ViolationDetail  string      `json:"violation_detail,omitempty"`
	PerNodeCommitIdx map[int]int `json:"per_node_commit_index"`
	PerNodeTerm      map[int]int `json:"per_node_term"`
	TotalEvents      int         `json:"total_events"`
}

type NodeStatus struct {
	ID          int    `json:"id"`
	State       string `json:"state"`
	Term        int    `json:"term"`
	CommitIndex int    `json:"commit_index"`
	LogLength   int    `json:"log_length"`
	Paused      bool   `json:"paused"`
	Crashed     bool   `json:"crashed"`
}

type ClientWriteRequest struct {
	Data string `json:"data"`
}

type ClientWriteResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type Peer struct {
	ID   int    `json:"id"`
	Addr string `json:"addr"`
}

type EventRecorder struct {
	startT time.Time
	events []TrackedEvent
}

func NewEventRecorder() *EventRecorder {
	return &EventRecorder{startT: time.Now()}
}

func (r *EventRecorder) Record(e TrackedEvent) {
	e.SimTimeMs = time.Since(r.startT).Milliseconds()
	r.events = append(r.events, e)
}

func (r *EventRecorder) Events() []TrackedEvent {
	cp := make([]TrackedEvent, len(r.events))
	copy(cp, r.events)
	return cp
}

func (r *EventRecorder) PersistEvents(path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range r.events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}
