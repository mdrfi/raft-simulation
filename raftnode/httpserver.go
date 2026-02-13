package raftnode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func NewHTTPServer(node *Node) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/control/write", cors(func(w http.ResponseWriter, r *http.Request) {
		var req ClientWriteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := node.HandleClientWrite(req.Data)
		writeJSON(w, resp)
	}))

	mux.HandleFunc("/control/pause", cors(func(w http.ResponseWriter, r *http.Request) {
		ms, _ := strconv.Atoi(r.URL.Query().Get("ms"))
		if ms <= 0 {
			ms = 2000
		}
		node.Pause(time.Duration(ms) * time.Millisecond)
		writeJSON(w, map[string]interface{}{"ok": true, "paused_ms": ms})
	}))

	mux.HandleFunc("/control/crash", cors(func(w http.ResponseWriter, r *http.Request) {
		node.Crash()
		writeJSON(w, map[string]interface{}{"ok": true, "crashed": true})
	}))

	mux.HandleFunc("/control/restart", cors(func(w http.ResponseWriter, r *http.Request) {
		node.Restart()
		writeJSON(w, map[string]interface{}{"ok": true, "restarted": true})
	}))

	mux.HandleFunc("/control/status", cors(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, node.Status())
	}))

	mux.HandleFunc("/control/events", cors(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, node.Events())
	}))

	mux.HandleFunc("/control/log", cors(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, node.GetLog())
	}))

	mux.HandleFunc("/control/persist", cors(func(w http.ResponseWriter, r *http.Request) {
		if err := node.PersistEvents(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true})
	}))

	mux.HandleFunc("/health", cors(func(w http.ResponseWriter, r *http.Request) {
		s := node.Status()
		if s.Crashed {
			http.Error(w, "crashed", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "ok")
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
