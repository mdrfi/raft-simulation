package raftnode

import (
	"encoding/json"
	"os"
)

func WriteHTML(path string, events []TrackedEvent, summary Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	evJSON, _ := json.Marshal(events)
	sumJSON, _ := json.Marshal(summary)

	w := func(s string) { f.WriteString(s) }

	w(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Raft Simulation Visualizer</title>
<style>
:root{--bg:#0d1117;--panel:#161b22;--border:#30363d;--text:#c9d1d9;--dim:#8b949e;--blue:#58a6ff;--green:#3fb950;--red:#f85149;--orange:#d29922;--purple:#bc8cff}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'Segoe UI',system-ui,sans-serif;background:var(--bg);color:var(--text);overflow-x:hidden}
header{background:var(--panel);border-bottom:1px solid var(--border);padding:12px 24px;display:flex;align-items:center;gap:16px;flex-wrap:wrap}
header h1{font-size:18px;color:var(--blue)}
header .stats{font-size:13px;color:var(--dim)}
header .stats b{color:var(--text)}

.controls{background:var(--panel);border-bottom:1px solid var(--border);padding:8px 24px;display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.controls label{font-size:13px;color:var(--dim)}
.controls input[type=range]{width:200px;accent-color:var(--blue)}
.controls button{background:var(--blue);border:none;color:#fff;padding:4px 14px;border-radius:4px;cursor:pointer;font-size:13px}
.controls button:hover{opacity:.85}
.controls .time-display{font-size:14px;font-variant-numeric:tabular-nums;color:var(--text);min-width:90px}

.main{display:flex;height:calc(100vh - 100px)}

.lanes{flex:1;overflow-y:auto;position:relative;padding:16px 24px}
.lane{display:flex;align-items:center;margin-bottom:2px;min-height:24px;position:relative}
.lane-label{width:80px;flex-shrink:0;font-size:13px;font-weight:600;color:var(--dim);text-align:right;padding-right:12px}
.lane-track{flex:1;position:relative;height:24px;background:var(--panel);border-radius:3px;overflow:visible}

.playhead{position:absolute;top:0;bottom:0;width:2px;background:var(--red);z-index:5;pointer-events:none;transition:none}
.playhead::after{content:'';position:absolute;top:-4px;left:-4px;width:10px;height:10px;background:var(--red);border-radius:50%}

.evt{position:absolute;top:2px;height:20px;border-radius:3px;cursor:pointer;transition:opacity .15s}
.evt:hover{opacity:.8!important;z-index:2}
.evt .tip{display:none;position:absolute;left:50%;top:110%;transform:translateX(-50%);background:#000;color:#fff;padding:4px 8px;border-radius:4px;font-size:11px;white-space:nowrap;z-index:10;pointer-events:none}
.evt:hover .tip{display:block}

.evt-election_start{background:var(--orange)}
.evt-vote_request{background:var(--purple);opacity:.6}
.evt-vote_granted{background:var(--purple)}
.evt-leader_elected{background:var(--green)}
.evt-heartbeat_sent{background:var(--blue);opacity:.3}
.evt-append_sent{background:var(--blue);opacity:.6}
.evt-append_acked{background:var(--blue)}
.evt-commit{background:var(--green);opacity:.9}
.evt-client_write{background:#fff;opacity:.7}
.evt-fault_applied{background:var(--red)}
.evt-node_crashed{background:var(--red)}
.evt-node_paused{background:var(--orange);opacity:.7}
.evt-node_resumed{background:var(--green);opacity:.5}
.evt-state_change{background:var(--dim);opacity:.4}

.sidebar{width:340px;background:var(--panel);border-left:1px solid var(--border);display:flex;flex-direction:column}
.sidebar h2{font-size:14px;padding:10px 14px;border-bottom:1px solid var(--border);color:var(--dim)}
.sidebar .log{flex:1;overflow-y:auto;padding:6px 10px;font-size:12px;font-family:'Cascadia Code',monospace}
.sidebar .log .le{padding:2px 0;border-bottom:1px solid var(--border)}
.sidebar .log .le .lt{color:var(--dim);min-width:58px;display:inline-block}

.legend{display:flex;gap:10px;flex-wrap:wrap;padding:4px 0}
.legend span{display:flex;align-items:center;gap:4px;font-size:11px;color:var(--dim)}
.legend .dot{width:10px;height:10px;border-radius:2px;flex-shrink:0}
</style>
</head>
<body>

<header>
  <h1>&#9783; Raft Simulation (HashiCorp)</h1>
  <div class="stats">
    Duration: <b id="s-dur"></b> ms &nbsp;|&nbsp;
    Leader changes: <b id="s-lc"></b> &nbsp;|&nbsp;
    Commits: <b id="s-cm"></b> &nbsp;|&nbsp;
    Events: <b id="s-ev"></b> &nbsp;|&nbsp;
    Safety: <b id="s-safe"></b>
  </div>
</header>

<div class="controls">
  <button id="btn-play">&#9654; Play</button>
  <button id="btn-reset">Reset</button>
  <label>Speed</label>
  <input type="range" id="speed" min="1" max="50" value="10">
  <div class="time-display">t = <span id="cur-t">0</span> ms</div>
  <div class="legend">
    <span><i class="dot" style="background:var(--orange)"></i>Election</span>
    <span><i class="dot" style="background:var(--purple)"></i>Vote</span>
    <span><i class="dot" style="background:var(--green)"></i>Leader/Commit</span>
    <span><i class="dot" style="background:var(--blue)"></i>Append/HB</span>
    <span><i class="dot" style="background:var(--red)"></i>Fault</span>
  </div>
</div>

<div class="main">
  <div class="lanes" id="lanes"></div>
  <div class="sidebar">
    <h2>Event Log</h2>
    <div class="log" id="logPanel"></div>
  </div>
</div>

<script>
const EVENTS = `)
	f.Write(evJSON)
	w(`;
const SUMMARY = `)
	f.Write(sumJSON)
	w(`;

document.getElementById("s-dur").textContent = SUMMARY.sim_duration_ms;
document.getElementById("s-lc").textContent  = SUMMARY.leader_changes;
document.getElementById("s-cm").textContent  = SUMMARY.total_commits;
document.getElementById("s-ev").textContent  = SUMMARY.total_events;
document.getElementById("s-safe").textContent = SUMMARY.safety_violation ? "VIOLATION: "+SUMMARY.violation_detail : "OK";
document.getElementById("s-safe").style.color = SUMMARY.safety_violation ? "var(--red)" : "var(--green)";

const maxT = SUMMARY.sim_duration_ms || 1;

const nodeSet = new Set();
EVENTS.forEach(e => { if(e.from>=0) nodeSet.add(e.from); if(e.to>=0) nodeSet.add(e.to); });
const nodeIDs = [...nodeSet].sort((a,b)=>a-b);

const lanesDiv = document.getElementById("lanes");
const tracks = {};
const playheads = {};
nodeIDs.forEach(id => {
  const lane = document.createElement("div"); lane.className = "lane";
  const lbl = document.createElement("div"); lbl.className = "lane-label"; lbl.textContent = "Node "+id;
  const track = document.createElement("div"); track.className = "lane-track";
  const ph = document.createElement("div"); ph.className = "playhead"; ph.style.left = "0%";
  track.appendChild(ph);
  lane.appendChild(lbl); lane.appendChild(track);
  lanesDiv.appendChild(lane);
  tracks[id] = track;
  playheads[id] = ph;
});

const markers = [];
EVENTS.forEach((e, idx) => {
  const nodeId = e.from >= 0 ? e.from : (e.to >= 0 ? e.to : null);
  if(nodeId === null) return;
  const track = tracks[nodeId]; if(!track) return;
  const pct = (e.sim_time_ms / maxT) * 100;
  const el = document.createElement("div");
  el.className = "evt evt-" + e.type;
  el.style.left = pct + "%";
  el.style.width = "6px";
  el.dataset.time = e.sim_time_ms;
  el.dataset.idx = idx;

  const tip = document.createElement("div"); tip.className = "tip";
  let tipText = "t="+e.sim_time_ms+" "+e.type;
  if(e.term) tipText += " term="+e.term;
  if(e.from>=0 && e.to>=0) tipText += " "+e.from+"\u2192"+e.to;
  if(e.entry_index) tipText += " idx="+e.entry_index;
  tip.textContent = tipText;
  el.appendChild(tip);

  track.appendChild(el);
  markers.push({el, time: e.sim_time_ms, idx});
});

let curTime = 0;
let playing = false;
let animFrame = null;
let visibleLogIdx = 0;

const curTSpan = document.getElementById("cur-t");
const logPanel = document.getElementById("logPanel");
const speedInput = document.getElementById("speed");
const btnPlay = document.getElementById("btn-play");

function updateView(t) {
  curTime = t;
  curTSpan.textContent = Math.floor(t);
  const pct = (t / maxT) * 100;
  for(const id of nodeIDs){ playheads[id].style.left = pct + "%"; }
  markers.forEach(m => { m.el.style.opacity = m.time <= t ? "" : "0.1"; });
  while(visibleLogIdx < EVENTS.length && EVENTS[visibleLogIdx].sim_time_ms <= t) {
    const e = EVENTS[visibleLogIdx];
    const div = document.createElement("div"); div.className = "le";
    let html = '<span class="lt">'+e.sim_time_ms+'ms</span> <b>'+e.type+'</b>';
    if(e.from>=0) html+=' n'+e.from;
    if(e.to>=0) html+='\u2192n'+e.to;
    if(e.term) html+=' T'+e.term;
    if(e.entry_index) html+=' i'+e.entry_index;
    div.innerHTML = html;
    logPanel.appendChild(div);
    visibleLogIdx++;
  }
  logPanel.scrollTop = logPanel.scrollHeight;
  btnPlay.textContent = playing ? "\u23F8 Pause" : "\u25B6 Play";
}

let lastFrameTime = 0;
function tick(ts) {
  if(!playing) return;
  if(!lastFrameTime) lastFrameTime = ts;
  const dt = ts - lastFrameTime;
  lastFrameTime = ts;
  const speed = parseInt(speedInput.value);
  curTime += dt * speed * 0.06;
  if(curTime >= maxT) { curTime = maxT; playing = false; }
  updateView(curTime);
  if(playing) animFrame = requestAnimationFrame(tick);
}

btnPlay.addEventListener("click", () => {
  if(curTime >= maxT) { resetView(); }
  playing = !playing;
  if(playing) { lastFrameTime = 0; animFrame = requestAnimationFrame(tick); }
  updateView(curTime);
});

function resetView() {
  playing = false;
  curTime = 0;
  visibleLogIdx = 0;
  logPanel.innerHTML = "";
  updateView(0);
}
document.getElementById("btn-reset").addEventListener("click", resetView);

updateView(0);
</script>
</body>
</html>
`)

	return nil
}
