package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"hashicorp-raft-demo/raftnode"
)

type nodeEndpoint struct {
	ID   int
	Addr string
	Host string
}

var (
	nodes   []nodeEndpoint
	dataDir string
)

func main() {
	dataDir = envStr("DATA_DIR", "/data")
	port := envStr("PORT", "9090")
	nodesStr := envStr("NODES", "")

	nodes = parseNodes(nodesStr)
	if len(nodes) == 0 {
		log.Fatal("no nodes configured – set NODES env var")
	}

	log.Printf("=== Raft Dashboard (HashiCorp) ===")
	log.Printf("Nodes: %d", len(nodes))
	for _, n := range nodes {
		log.Printf("  node-%d @ %s (host: %s)", n.ID, n.Addr, n.Host)
	}

	waitForNodes(nodes, 120*time.Second)
	log.Printf("All nodes healthy – dashboard ready")

	mux := http.NewServeMux()

	mux.HandleFunc("/api/nodes", handleGetNodes)
	mux.HandleFunc("/api/status", handleGetAllStatus)
	mux.HandleFunc("/api/crash", handleCrash)
	mux.HandleFunc("/api/pause", handlePause)
	mux.HandleFunc("/api/restart", handleRestart)
	mux.HandleFunc("/api/write", handleWrite)
	mux.HandleFunc("/api/collect", handleCollect)
	mux.HandleFunc("/api/node-events", handleNodeEvents)
	mux.HandleFunc("/api/node-log", handleNodeLog)

	mux.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir(dataDir))))

	mux.HandleFunc("/", handleDashboard)

	addr := ":" + port
	log.Printf("Dashboard at http://0.0.0.0%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func apiJSON(w http.ResponseWriter, v interface{}) {
	corsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handleGetNodes(w http.ResponseWriter, r *http.Request) {
	type nodeInfo struct {
		ID   int    `json:"id"`
		Addr string `json:"addr"`
		Host string `json:"host"`
	}
	out := make([]nodeInfo, len(nodes))
	for i, n := range nodes {
		out[i] = nodeInfo{ID: n.ID, Addr: n.Addr, Host: n.Host}
	}
	apiJSON(w, out)
}

func handleGetAllStatus(w http.ResponseWriter, r *http.Request) {
	type statusResult struct {
		raftnode.NodeStatus
		Healthy bool `json:"healthy"`
	}
	results := make([]statusResult, len(nodes))
	for i, n := range nodes {
		s, healthy := getStatusSafe(n.Addr)
		results[i] = statusResult{NodeStatus: s, Healthy: healthy}
		if !healthy {
			results[i].ID = n.ID
		}
	}
	apiJSON(w, results)
}

func handleCrash(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	n := findNode(id)
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	resp := postJSON(n.Addr, "/control/crash", nil)
	w.Header().Set("Content-Type", "application/json")
	corsHeaders(w)
	w.Write(resp)
}

func handlePause(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	ms := r.URL.Query().Get("ms")
	if ms == "" {
		ms = "3000"
	}
	n := findNode(id)
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	resp := postJSON(n.Addr, "/control/pause?ms="+ms, nil)
	w.Header().Set("Content-Type", "application/json")
	corsHeaders(w)
	w.Write(resp)
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	n := findNode(id)
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	resp := postJSON(n.Addr, "/control/restart", nil)
	w.Header().Set("Content-Type", "application/json")
	corsHeaders(w)
	w.Write(resp)
}

func handleWrite(w http.ResponseWriter, r *http.Request) {
	data := r.URL.Query().Get("data")
	if data == "" {
		data = fmt.Sprintf("manual-write-%d", time.Now().UnixMilli())
	}
	targetID := r.URL.Query().Get("id")

	var result []byte
	if targetID != "" {

		id, _ := strconv.Atoi(targetID)
		n := findNode(id)
		if n == nil {
			http.Error(w, "node not found", 404)
			return
		}
		result = postJSON(n.Addr, "/control/write", raftnode.ClientWriteRequest{Data: data})
	} else {

		for _, n := range nodes {
			resp := postJSON(n.Addr, "/control/write", raftnode.ClientWriteRequest{Data: data})
			var wr raftnode.ClientWriteResponse
			json.Unmarshal(resp, &wr)
			if wr.OK {
				result = resp
				break
			}
		}
		if result == nil {
			result = []byte(`{"ok":false,"error":"no leader found"}`)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	corsHeaders(w)
	w.Write(result)
}

func handleNodeEvents(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	n := findNode(id)
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/control/events", n.Addr))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	corsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func handleNodeLog(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	n := findNode(id)
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/control/log", n.Addr))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	corsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func handleCollect(w http.ResponseWriter, r *http.Request) {

	for _, n := range nodes {
		postJSON(n.Addr, "/control/persist", nil)
	}

	allEvents := collectEvents(nodes)
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].SimTimeMs < allEvents[j].SimTimeMs
	})

	statuses := collectStatuses(nodes)
	var durMs int64
	if len(allEvents) > 0 {
		durMs = allEvents[len(allEvents)-1].SimTimeMs
	}
	summary := buildSummary(allEvents, statuses, durMs)

	os.MkdirAll(dataDir, 0o755)

	evPath := dataDir + "/events.jsonl"
	writeEventsJSONL(evPath, allEvents)

	sumPath := dataDir + "/summary.json"
	writeSummaryJSON(sumPath, summary)

	htmlPath := dataDir + "/timeline.html"
	raftnode.WriteHTML(htmlPath, allEvents, summary)

	log.Printf("Collected %d events → timeline.html", len(allEvents))

	apiJSON(w, map[string]interface{}{
		"ok":             true,
		"total_events":   len(allEvents),
		"timeline_url":   "/data/timeline.html",
		"events_url":     "/data/events.jsonl",
		"summary_url":    "/data/summary.json",
		"leader_changes": summary.LeaderChanges,
		"total_commits":  summary.TotalCommits,
		"safety_ok":      !summary.SafetyViolation,
	})
}

//go:embed dashboard.html
var dashboardHTML []byte

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

func waitForNodes(nodes []nodeEndpoint, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for _, n := range nodes {
		for {
			if time.Now().After(deadline) {
				log.Fatalf("timeout waiting for node-%d at %s", n.ID, n.Addr)
			}
			resp, err := client.Get(fmt.Sprintf("http://%s/health", n.Addr))
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				log.Printf("node-%d healthy", n.ID)
				break
			}
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func postJSON(addr, path string, body interface{}) []byte {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}
	resp, err := http.Post(fmt.Sprintf("http://%s%s", addr, path), "application/json", bodyReader)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	result, _ := io.ReadAll(resp.Body)
	return result
}

func getStatusSafe(addr string) (raftnode.NodeStatus, bool) {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/control/status", addr))
	if err != nil {
		return raftnode.NodeStatus{}, false
	}
	defer resp.Body.Close()
	var s raftnode.NodeStatus
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return raftnode.NodeStatus{}, false
	}
	return s, true
}

func collectEvents(nodes []nodeEndpoint) []raftnode.TrackedEvent {
	var all []raftnode.TrackedEvent
	for _, n := range nodes {
		resp, err := http.Get(fmt.Sprintf("http://%s/control/events", n.Addr))
		if err != nil {
			log.Printf("warning: cannot collect events from node-%d: %v", n.ID, err)
			continue
		}
		var events []raftnode.TrackedEvent
		json.NewDecoder(resp.Body).Decode(&events)
		resp.Body.Close()
		all = append(all, events...)
	}
	return all
}

func collectStatuses(nodes []nodeEndpoint) []raftnode.NodeStatus {
	var ss []raftnode.NodeStatus
	for _, n := range nodes {
		s, _ := getStatusSafe(n.Addr)
		ss = append(ss, s)
	}
	return ss
}

func findNode(id int) *nodeEndpoint {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

func buildSummary(events []raftnode.TrackedEvent, statuses []raftnode.NodeStatus, durMs int64) raftnode.Summary {
	s := raftnode.Summary{
		SimDurationMs:    durMs,
		PerNodeCommitIdx: make(map[int]int),
		PerNodeTerm:      make(map[int]int),
		TotalEvents:      len(events),
	}
	for _, e := range events {
		switch e.Type {
		case "leader_elected":
			s.LeaderChanges++
		case "commit":
			s.TotalCommits++
		}
	}
	for _, st := range statuses {
		s.PerNodeCommitIdx[st.ID] = st.CommitIndex
		s.PerNodeTerm[st.ID] = st.Term
	}
	return s
}

func writeEventsJSONL(path string, events []raftnode.TrackedEvent) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("error writing events: %v", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		enc.Encode(e)
	}
}

func writeSummaryJSON(path string, s raftnode.Summary) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("error writing summary: %v", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(s)
}

func parseNodes(s string) []nodeEndpoint {
	var nodes []nodeEndpoint
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		id, _ := strconv.Atoi(part[:eqIdx])
		addr := part[eqIdx+1:]

		host := addr
		if strings.HasPrefix(addr, "node-") {
			host = fmt.Sprintf("localhost:%d", 8080+id)
		}
		nodes = append(nodes, nodeEndpoint{ID: id, Addr: addr, Host: host})
	}
	return nodes
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
