import { useState, useEffect, useCallback, useRef } from "react";

// --- Theme ---
const theme = {
  bg: "#0e1117",
  surface: "#161b22",
  surfaceHover: "#1c2333",
  border: "#27303f",
  borderSubtle: "#1e2536",
  text: "#e2e8f0",
  textMuted: "#7b8ba3",
  textDim: "#4a5568",
  accent: "#22d3ee",
  accentDim: "rgba(34,211,238,0.12)",
  accentGlow: "rgba(34,211,238,0.25)",
  success: "#10b981",
  warning: "#f59e0b",
  warningDim: "rgba(245,158,11,0.12)",
  error: "#ef4444",
  errorDim: "rgba(239,68,68,0.12)",
  purple: "#a78bfa",
};

const font = `'DM Sans', system-ui, -apple-system, sans-serif`;
const mono = `'JetBrains Mono', 'Fira Code', monospace`;

// --- API helpers ---
const API_KEY = new URLSearchParams(window.location.search).get("apikey") || "";

async function apiFetch(path, opts = {}) {
  const headers = { ...opts.headers };
  if (API_KEY) headers["X-Api-Key"] = API_KEY;
  const res = await fetch(path, { ...opts, headers });
  return res.json();
}

async function apiPost(path, body) {
  return apiFetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

async function apiDelete(path) {
  return apiFetch(path, { method: "DELETE" });
}

// --- Utilities ---
function formatBytes(bytes) {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function formatSpeed(bps) {
  if (!bps || bps === 0) return "—";
  return formatBytes(bps) + "/s";
}

function formatETA(remainingBytes, speedBps) {
  if (!speedBps || speedBps <= 0 || !remainingBytes) return "—";
  const secs = remainingBytes / speedBps;
  if (secs < 60) return `${Math.round(secs)}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ${Math.round(secs % 60)}s`;
  const hrs = Math.floor(secs / 3600);
  const mins = Math.floor((secs % 3600) / 60);
  return `${hrs}h ${mins}m`;
}

function timeAgo(dateStr) {
  if (!dateStr) return "";
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

// --- Icons (inline SVG) ---
const Icons = {
  Download: () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>,
  Pause: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/></svg>,
  Play: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>,
  Trash: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>,
  Clock: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>,
  Check: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><polyline points="20 6 9 17 4 12"/></svg>,
  X: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>,
  Cpu: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/></svg>,
  Activity: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>,
  Upload: () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>,
};

// --- Status badge ---
function StatusBadge({ status }) {
  const config = {
    downloading: { color: theme.accent, bg: theme.accentDim, label: "Downloading" },
    queued: { color: theme.textMuted, bg: "rgba(123,139,163,0.1)", label: "Queued" },
    "post-processing": { color: theme.purple, bg: "rgba(167,139,250,0.12)", label: "Processing" },
    paused: { color: theme.warning, bg: theme.warningDim, label: "Paused" },
    completed: { color: theme.success, bg: "rgba(16,185,129,0.12)", label: "Completed" },
    failed: { color: theme.error, bg: theme.errorDim, label: "Failed" },
  }[status] || { color: theme.textMuted, bg: "transparent", label: status };

  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: 5,
      padding: "3px 10px", borderRadius: 20,
      fontSize: 11, fontWeight: 600, letterSpacing: "0.03em",
      color: config.color, background: config.bg, textTransform: "uppercase",
    }}>
      {status === "downloading" && <span style={{ width: 6, height: 6, borderRadius: "50%", background: config.color, animation: "pulse 1.5s ease-in-out infinite" }} />}
      {config.label}
    </span>
  );
}

// --- Progress bar ---
function ProgressBar({ progress, status }) {
  const color = status === "paused" ? theme.warning :
                status === "post-processing" ? theme.purple :
                status === "failed" ? theme.error : theme.accent;
  return (
    <div style={{ width: "100%", height: 4, background: theme.border, borderRadius: 2, overflow: "hidden" }}>
      <div style={{
        width: `${Math.min(progress, 100)}%`, height: "100%",
        background: `linear-gradient(90deg, ${color}, ${color}dd)`,
        borderRadius: 2, transition: "width 0.8s cubic-bezier(0.22, 1, 0.36, 1)",
        ...(status === "downloading" ? { boxShadow: `0 0 8px ${color}40` } : {}),
      }} />
    </div>
  );
}

// --- Category tag ---
function CategoryTag({ category }) {
  if (!category) return null;
  const colors = { movies: "#f472b6", tv: "#60a5fa", music: "#34d399", software: "#fbbf24" };
  return (
    <span style={{ fontSize: 10, fontWeight: 700, letterSpacing: "0.06em", color: colors[category] || theme.textMuted, textTransform: "uppercase", opacity: 0.9 }}>
      {category}
    </span>
  );
}

// --- Speed sparkline ---
function SpeedChart({ data }) {
  const max = Math.max(...data, 1);
  const w = 160, h = 32;
  const points = data.map((v, i) => `${(i / (data.length - 1)) * w},${h - (v / max) * h}`).join(" ");
  const fillPoints = `0,${h} ${points} ${w},${h}`;
  return (
    <svg width={w} height={h} style={{ display: "block" }}>
      <defs>
        <linearGradient id="speedGrad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={theme.accent} stopOpacity="0.3" />
          <stop offset="100%" stopColor={theme.accent} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={fillPoints} fill="url(#speedGrad)" />
      <polyline points={points} fill="none" stroke={theme.accent} strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

// --- Queue item ---
function QueueItem({ job, onPause, onResume, onRemove }) {
  const [hovered, setHovered] = useState(false);
  const [expanded, setExpanded] = useState(false);
  return (
    <div
      onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
      style={{
        padding: "16px 20px", background: hovered ? theme.surfaceHover : "transparent",
        borderBottom: `1px solid ${theme.borderSubtle}`, transition: "background 0.15s ease", cursor: "pointer",
      }}
      onClick={() => setExpanded(!expanded)}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 4 }}>
            <CategoryTag category={job.category} />
            <StatusBadge status={job.status} />
          </div>
          <div style={{ fontSize: 14, fontWeight: 500, color: theme.text, fontFamily: mono, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", maxWidth: 500 }}>
            {job.name}
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 6, opacity: hovered ? 1 : 0, transition: "opacity 0.15s ease" }}
             onClick={e => e.stopPropagation()}>
          {job.status === "paused" ? (
            <ActionBtn icon={<Icons.Play />} title="Resume" onClick={() => onResume(job.id)} />
          ) : (job.status === "downloading" || job.status === "queued") ? (
            <ActionBtn icon={<Icons.Pause />} title="Pause" onClick={() => onPause(job.id)} />
          ) : null}
          <ActionBtn icon={<Icons.Trash />} title="Remove" onClick={() => onRemove(job.id)} color={theme.error} />
        </div>
      </div>
      <ProgressBar progress={job.progress} status={job.status} />
      <div style={{ display: "flex", alignItems: "center", gap: 16, marginTop: 8, fontSize: 12, color: theme.textMuted, fontFamily: mono }}>
        <span>{(job.progress || 0).toFixed(1)}%</span>
        <span>{formatBytes(job.downloaded_bytes)} / {formatBytes(job.total_bytes)}</span>
        <span style={{ display: "flex", alignItems: "center", gap: 4 }}><Icons.Clock /> {timeAgo(job.added_at)}</span>
        <span style={{ marginLeft: "auto", fontSize: 11 }}>
          {job.done_segments}/{job.total_segments} seg
          {job.failed_segments > 0 && <span style={{ color: theme.error, marginLeft: 4 }}>({job.failed_segments} failed)</span>}
        </span>
      </div>
      {expanded && (
        <div style={{ marginTop: 12, padding: 12, background: theme.bg, borderRadius: 8, fontSize: 12, fontFamily: mono, color: theme.textMuted }}
             onClick={e => e.stopPropagation()}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "6px 24px" }}>
            <div><span style={{ color: theme.textDim }}>ID:</span> <span style={{ color: theme.text }}>{job.id}</span></div>
            <div><span style={{ color: theme.textDim }}>Priority:</span> <span style={{ color: theme.text }}>{job.priority}</span></div>
            <div><span style={{ color: theme.textDim }}>Added:</span> <span style={{ color: theme.text }}>{job.added_at ? new Date(job.added_at).toLocaleString() : "—"}</span></div>
            <div><span style={{ color: theme.textDim }}>Started:</span> <span style={{ color: theme.text }}>{job.started_at ? new Date(job.started_at).toLocaleString() : "—"}</span></div>
            <div><span style={{ color: theme.textDim }}>Segments:</span> <span style={{ color: theme.text }}>{job.done_segments} / {job.total_segments} done</span>{job.failed_segments > 0 && <span style={{ color: theme.error }}> ({job.failed_segments} failed)</span>}</div>
            <div><span style={{ color: theme.textDim }}>Downloaded:</span> <span style={{ color: theme.text }}>{formatBytes(job.downloaded_bytes)} / {formatBytes(job.total_bytes)}</span></div>
            {job.nzb_path && <div style={{ gridColumn: "1 / -1" }}><span style={{ color: theme.textDim }}>NZB:</span> <span style={{ color: theme.text }}>{job.nzb_path}</span></div>}
            {job.error && <div style={{ gridColumn: "1 / -1", color: theme.error }}>Error: {job.error}</div>}
          </div>
        </div>
      )}
    </div>
  );
}

function ActionBtn({ icon, title, onClick, color }) {
  const [h, setH] = useState(false);
  return (
    <button title={title} onClick={onClick} onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)}
      style={{
        display: "flex", alignItems: "center", justifyContent: "center", width: 32, height: 32, borderRadius: 8,
        border: `1px solid ${h ? (color || theme.accent) : theme.border}`,
        background: h ? (color || theme.accent) + "18" : "transparent",
        color: h ? (color || theme.accent) : theme.textMuted, cursor: "pointer", transition: "all 0.15s ease",
      }}
    >{icon}</button>
  );
}

// --- History item ---
function HistoryItem({ item, onDelete }) {
  const [hovered, setHovered] = useState(false);
  return (
    <div onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
      style={{ display: "flex", alignItems: "center", gap: 16, padding: "12px 20px", borderBottom: `1px solid ${theme.borderSubtle}`, fontSize: 13, background: hovered ? theme.surfaceHover : "transparent", transition: "background 0.15s ease" }}>
      <div style={{ width: 20, display: "flex", justifyContent: "center" }}>
        {item.status === "completed" ? <span style={{ color: theme.success }}><Icons.Check /></span> : <span style={{ color: theme.error }}><Icons.X /></span>}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: mono, fontSize: 13, color: theme.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{item.name}</div>
        <div style={{ display: "flex", gap: 12, marginTop: 3, fontSize: 11, color: theme.textMuted }}>
          <CategoryTag category={item.category} />
          <span>{formatBytes(item.total_bytes)}</span>
          {item.duration && <span>{item.duration}</span>}
          {item.error && <span style={{ color: theme.error }}>{item.error}</span>}
        </div>
      </div>
      <div style={{ opacity: hovered ? 1 : 0, transition: "opacity 0.15s ease" }}>
        <ActionBtn icon={<Icons.Trash />} title="Delete" onClick={() => onDelete(item.id)} color={theme.error} />
      </div>
    </div>
  );
}

// --- Stat card ---
function StatCard({ label, value, sub, icon, accent }) {
  return (
    <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, padding: "16px 20px", display: "flex", flexDirection: "column", gap: 4, position: "relative", overflow: "hidden" }}>
      <div style={{ position: "absolute", top: -8, right: -8, width: 48, height: 48, borderRadius: "50%", background: (accent || theme.accent) + "0a", display: "flex", alignItems: "center", justifyContent: "center", color: accent || theme.accent, opacity: 0.5 }}>{icon}</div>
      <span style={{ fontSize: 11, color: theme.textMuted, textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 600 }}>{label}</span>
      <span style={{ fontSize: 24, fontWeight: 700, color: theme.text, fontFamily: mono, letterSpacing: "-0.02em" }}>{value}</span>
      {sub && <span style={{ fontSize: 11, color: theme.textDim }}>{sub}</span>}
    </div>
  );
}

// --- NZB Upload Dialog ---
function UploadDialog({ onClose, onUploaded }) {
  const fileRef = useRef(null);
  const [category, setCategory] = useState("");
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");

  const handleUpload = async () => {
    const file = fileRef.current?.files[0];
    if (!file) return;
    setUploading(true);
    setError("");
    const form = new FormData();
    form.append("nzbfile", file);
    if (category) form.append("category", category);
    try {
      const headers = {};
      if (API_KEY) headers["X-Api-Key"] = API_KEY;
      const res = await fetch("/api/nzb/upload", { method: "POST", headers, body: form });
      const data = await res.json();
      if (!res.ok) { setError(data.error || "Upload failed"); }
      else { onUploaded(); onClose(); }
    } catch (e) { setError(e.message); }
    finally { setUploading(false); }
  };

  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.6)", zIndex: 200, display: "flex", alignItems: "center", justifyContent: "center" }}>
      <div onClick={e => e.stopPropagation()} style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 16, padding: 28, width: 420, maxWidth: "90vw" }}>
        <h3 style={{ margin: "0 0 16px", fontSize: 16, fontWeight: 600 }}>Upload NZB</h3>
        <input ref={fileRef} type="file" accept=".nzb" style={{ width: "100%", padding: 8, marginBottom: 12, background: theme.bg, border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.text, fontFamily: font }} />
        <input value={category} onChange={e => setCategory(e.target.value)} placeholder="Category (optional)"
          style={{ width: "100%", padding: "8px 12px", marginBottom: 16, background: theme.bg, border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.text, fontFamily: font, fontSize: 13, boxSizing: "border-box" }} />
        {error && <div style={{ color: theme.error, fontSize: 12, marginBottom: 12 }}>{error}</div>}
        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button onClick={onClose} style={{ padding: "8px 16px", borderRadius: 8, border: `1px solid ${theme.border}`, background: "transparent", color: theme.textMuted, cursor: "pointer", fontFamily: font }}>Cancel</button>
          <button onClick={handleUpload} disabled={uploading} style={{ padding: "8px 16px", borderRadius: 8, border: "none", background: theme.accent, color: theme.bg, cursor: "pointer", fontWeight: 600, fontFamily: font, opacity: uploading ? 0.5 : 1 }}>
            {uploading ? "Uploading..." : "Upload"}
          </button>
        </div>
      </div>
    </div>
  );
}

// --- Main App ---
export default function GoBinUI() {
  const [tab, setTab] = useState("queue");
  const [queueData, setQueueData] = useState([]);
  const [historyData, setHistoryData] = useState([]);
  const [status, setStatus] = useState({});
  const [queuePaused, setQueuePaused] = useState(false);
  const [showUpload, setShowUpload] = useState(false);
  const [speedHistory, setSpeedHistory] = useState(() => new Array(30).fill(0));

  const fetchQueue = useCallback(async () => {
    try {
      const data = await apiFetch("/api/queue");
      setQueueData(data.queue || []);
      setQueuePaused(data.paused || false);
    } catch (e) { /* ignore */ }
  }, []);

  const fetchStatus = useCallback(async () => {
    try { setStatus(await apiFetch("/api/status")); } catch (e) { /* ignore */ }
  }, []);

  const fetchHistory = useCallback(async () => {
    try {
      const data = await apiFetch("/api/history");
      setHistoryData(data.history || []);
    } catch (e) { /* ignore */ }
  }, []);

  useEffect(() => { fetchQueue(); fetchStatus(); fetchHistory(); }, [fetchQueue, fetchStatus, fetchHistory]);

  // SSE for live updates, with polling fallback
  useEffect(() => {
    let lastSSEUpdate = 0;

    const applyUpdate = (data) => {
      setQueueData(data.queue || []);
      setQueuePaused(data.paused || false);
      setStatus(prev => ({
        ...prev,
        speed_bps: data.speed_bps ?? prev.speed_bps,
        uptime_secs: data.uptime_secs ?? prev.uptime_secs,
        version: data.version ?? prev.version,
      }));
      setSpeedHistory(prev => [...prev.slice(1), data.speed_bps || 0]);
    };

    // SSE connection
    const es = new EventSource("/api/events");
    es.addEventListener("queue", (e) => {
      lastSSEUpdate = Date.now();
      try { applyUpdate(JSON.parse(e.data)); } catch (err) { /* ignore */ }
    });

    // Polling fallback — only runs when SSE hasn't sent data recently
    const poll = setInterval(async () => {
      if (Date.now() - lastSSEUpdate < 3000) return; // SSE is working, skip
      try {
        const [q, s] = await Promise.all([apiFetch("/api/queue"), apiFetch("/api/status")]);
        applyUpdate({ ...q, ...s });
      } catch (e) { /* ignore */ }
    }, 2000);

    return () => { es.close(); clearInterval(poll); };
  }, []);

  const handlePause = async (id) => { await apiPost(`/api/queue/${id}/pause`); fetchQueue(); };
  const handleResume = async (id) => { await apiPost(`/api/queue/${id}/resume`); fetchQueue(); };
  const handleRemove = async (id) => { await apiDelete(`/api/queue/${id}`); fetchQueue(); };
  const handlePauseAll = async () => { await apiPost("/api/queue/pause"); fetchQueue(); };
  const handleResumeAll = async () => { await apiPost("/api/queue/resume"); fetchQueue(); };
  const handleDeleteHistory = async (id) => { await apiDelete(`/api/history/${id}`); fetchHistory(); };
  const handleClearHistory = async () => { await apiDelete("/api/history"); fetchHistory(); };

  const activeCount = queueData.filter(j => j.status === "downloading").length;
  const remainingBytes = queueData.filter(j => j.status !== "completed").reduce((a, j) => a + ((j.total_bytes || 0) - (j.downloaded_bytes || 0)), 0);
  const speedBps = status.speed_bps || 0;

  return (
    <div style={{ minHeight: "100vh", background: theme.bg, color: theme.text, fontFamily: font, lineHeight: 1.5 }}>
      <style>{`
        @import url('https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700&family=JetBrains+Mono:wght@400;500;600&display=swap');
        * { margin: 0; padding: 0; box-sizing: border-box; }
        ::-webkit-scrollbar { width: 6px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: ${theme.border}; border-radius: 3px; }
        button { font-family: inherit; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
        @keyframes slideIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
      `}</style>

      {showUpload && <UploadDialog onClose={() => setShowUpload(false)} onUploaded={fetchQueue} />}

      <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 28px", height: 56, borderBottom: `1px solid ${theme.border}`, background: theme.surface, position: "sticky", top: 0, zIndex: 100 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ width: 32, height: 32, borderRadius: 8, background: `linear-gradient(135deg, ${theme.accent}, ${theme.purple})`, display: "flex", alignItems: "center", justifyContent: "center", boxShadow: `0 2px 12px ${theme.accentGlow}` }}>
            <Icons.Download />
          </div>
          <span style={{ fontSize: 18, fontWeight: 700, letterSpacing: "-0.02em" }}>Go<span style={{ color: theme.accent }}>Bin</span></span>
          <span style={{ fontSize: 10, color: theme.textDim, fontFamily: mono, marginLeft: 4 }}>{status.version || ""}</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <SpeedChart data={speedHistory} />
          <div style={{ textAlign: "right", minWidth: 80 }}>
            <div style={{ fontSize: 14, fontWeight: 700, fontFamily: mono, color: speedBps > 0 ? theme.accent : theme.textDim }}>
              {formatSpeed(speedBps)}
            </div>
            <div style={{ fontSize: 10, color: theme.textDim }}>
              {activeCount > 0 ? `${activeCount} active` : "idle"}
            </div>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          {queuePaused
            ? <HeaderBtn icon={<Icons.Play />} label="Resume Queue" onClick={handleResumeAll} accent />
            : <HeaderBtn icon={<Icons.Pause />} label="Pause Queue" onClick={handlePauseAll} />}
          <HeaderBtn icon={<Icons.Upload />} label="Add NZB" onClick={() => setShowUpload(true)} accent />
        </div>
      </header>

      <nav style={{ display: "flex", borderBottom: `1px solid ${theme.border}`, background: theme.surface, padding: "0 28px" }}>
        {[{ key: "queue", label: "Queue", count: queueData.length }, { key: "history", label: "History", count: historyData.length }, { key: "config", label: "Config" }].map(t => (
          <button key={t.key} onClick={() => { setTab(t.key); if (t.key === "history") fetchHistory(); }}
            style={{ padding: "12px 20px", fontSize: 13, fontWeight: 600, color: tab === t.key ? theme.text : theme.textMuted, background: "none", border: "none", cursor: "pointer", borderBottom: `2px solid ${tab === t.key ? theme.accent : "transparent"}`, transition: "all 0.15s ease", display: "flex", alignItems: "center", gap: 8 }}>
            {t.label}
            {t.count != null && <span style={{ fontSize: 10, fontWeight: 700, fontFamily: mono, padding: "1px 7px", borderRadius: 10, background: tab === t.key ? theme.accentDim : theme.borderSubtle, color: tab === t.key ? theme.accent : theme.textDim }}>{t.count}</span>}
          </button>
        ))}
      </nav>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12, padding: "20px 28px" }}>
        <StatCard label="Speed" value={formatSpeed(speedBps)} sub={`${activeCount} active`} icon={<Icons.Activity />} accent={theme.accent} />
        <StatCard label="Queue" value={queueData.length} sub={`${formatBytes(remainingBytes)} remaining`} icon={<Icons.Download />} accent={theme.accent} />
        <StatCard label="ETA" value={formatETA(remainingBytes, speedBps)} sub={queuePaused ? "Paused" : ""} icon={<Icons.Clock />} accent={theme.purple} />
        <StatCard label="Status" value={queuePaused ? "Paused" : "Active"} sub={`v${status.version || "?"} · ${status.uptime_secs || 0}s uptime`} icon={<Icons.Cpu />} accent={queuePaused ? theme.warning : theme.success} />
      </div>

      <div style={{ padding: "0 28px 28px" }}>
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: "hidden" }}>
          {tab === "queue" && (<>
            {queueData.map((job, i) => (
              <div key={job.id} style={{ animation: `slideIn 0.3s ease ${i * 0.05}s both` }}>
                <QueueItem job={job} onPause={handlePause} onResume={handleResume} onRemove={handleRemove} />
              </div>
            ))}
            {queueData.length === 0 && (
              <div style={{ padding: 60, textAlign: "center", color: theme.textDim }}>
                <div style={{ fontSize: 14, marginBottom: 4 }}>Queue is empty</div>
                <div style={{ fontSize: 12 }}>Upload an NZB file to get started</div>
              </div>
            )}
          </>)}
          {tab === "history" && (<>
            {historyData.length > 0 && (
              <div style={{ display: "flex", justifyContent: "flex-end", padding: "8px 20px", borderBottom: `1px solid ${theme.borderSubtle}` }}>
                <button onClick={handleClearHistory} style={{ fontSize: 11, color: theme.error, background: "transparent", border: "none", cursor: "pointer", fontFamily: font, opacity: 0.7 }}>
                  Clear All History
                </button>
              </div>
            )}
            {historyData.map((item, i) => (
              <div key={item.id} style={{ animation: `slideIn 0.3s ease ${i * 0.05}s both` }}>
                <HistoryItem item={item} onDelete={handleDeleteHistory} />
              </div>
            ))}
            {historyData.length === 0 && (
              <div style={{ padding: 60, textAlign: "center", color: theme.textDim }}><div style={{ fontSize: 14 }}>No history yet</div></div>
            )}
          </>)}
          {tab === "config" && <ConfigEditor />}
        </div>
      </div>
    </div>
  );
}

// --- API Key Manager ---
function APIKeyManager() {
  const [apiKey, setApiKey] = useState("");
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);
  const [rolling, setRolling] = useState(false);

  const fetchKey = useCallback(async () => {
    try {
      const data = await apiFetch("/api/apikey");
      setApiKey(data.api_key || "");
    } catch (e) { /* ignore */ }
  }, []);

  useEffect(() => { fetchKey(); }, [fetchKey]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(apiKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (e) { /* fallback */ }
  };

  const handleRoll = async () => {
    if (!confirm("Generate a new API key? The old key will stop working immediately. Any external clients (Sonarr, Radarr, etc.) will need to be updated.")) return;
    setRolling(true);
    try {
      const res = await fetch("/api/apikey/roll", {
        method: "POST",
        headers: API_KEY ? { "X-Api-Key": API_KEY } : {},
      });
      const data = await res.json();
      if (!res.ok || !data.api_key) {
        alert("Failed to roll API key: " + (data.error || `HTTP ${res.status}`));
        return;
      }
      setApiKey(data.api_key);
      setRevealed(true);
      if (data.warning) alert(data.warning);
    } catch (e) {
      alert("Failed to roll API key: " + e.message);
    } finally { setRolling(false); }
  };

  const masked = apiKey ? apiKey.slice(0, 6) + "••••••••••••••••••••••••" + apiKey.slice(-4) : "";

  return (
    <div style={{ marginBottom: 10 }}>
      <label style={labelStyle}>API Key</label>
      <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <input type="text" readOnly value={revealed ? apiKey : masked}
          style={{ ...inputStyle, flex: 1, fontFamily: mono, fontSize: 12, letterSpacing: "0.03em" }}
          onClick={() => revealed && handleCopy()} />
        <button onClick={() => setRevealed(!revealed)} style={{
          padding: "8px 12px", borderRadius: 8, border: `1px solid ${theme.border}`,
          background: "transparent", color: theme.textMuted, cursor: "pointer", fontSize: 11, fontFamily: font, whiteSpace: "nowrap",
        }}>{revealed ? "Hide" : "Reveal"}</button>
        <button onClick={handleCopy} style={{
          padding: "8px 12px", borderRadius: 8, border: `1px solid ${theme.border}`,
          background: "transparent", color: copied ? theme.success : theme.textMuted,
          cursor: "pointer", fontSize: 11, fontFamily: font, whiteSpace: "nowrap",
        }}>{copied ? "Copied!" : "Copy"}</button>
        <button onClick={handleRoll} disabled={rolling} style={{
          padding: "8px 12px", borderRadius: 8, border: "none",
          background: theme.warningDim, color: theme.warning,
          cursor: "pointer", fontSize: 11, fontFamily: font, fontWeight: 600, whiteSpace: "nowrap",
          opacity: rolling ? 0.5 : 1,
        }}>{rolling ? "Rolling..." : "Roll Key"}</button>
      </div>
      <div style={{ fontSize: 10, color: theme.textDim, marginTop: 4 }}>
        Used by external clients (Sonarr, Radarr, etc.) to authenticate API requests.
      </div>
    </div>
  );
}

// --- Config Editor ---
const inputStyle = { width: "100%", padding: "8px 12px", background: theme.bg, border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.text, fontFamily: font, fontSize: 13, boxSizing: "border-box" };
const labelStyle = { fontSize: 11, color: theme.textMuted, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", marginBottom: 4, display: "block" };
const sectionStyle = { marginBottom: 20, padding: 16, background: theme.bg, borderRadius: 8, border: `1px solid ${theme.border}` };
const checkboxStyle = { accentColor: theme.accent, marginRight: 8 };

function ConfigField({ label, value, onChange, type = "text", placeholder = "" }) {
  if (type === "checkbox") {
    return <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: theme.text, cursor: "pointer", marginBottom: 8 }}>
      <input type="checkbox" checked={!!value} onChange={e => onChange(e.target.checked)} style={checkboxStyle} />{label}
    </label>;
  }
  return <div style={{ marginBottom: 10 }}>
    <label style={labelStyle}>{label}</label>
    <input type={type} value={value || ""} onChange={e => onChange(type === "number" ? parseInt(e.target.value) || 0 : e.target.value)} placeholder={placeholder} style={inputStyle} />
  </div>;
}

function ServerCard({ srv, index, update, removeServer }) {
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState(null);

  const handleTest = async () => {
    setTesting(true);
    setResult(null);
    try {
      const res = await fetch("/api/servers/test", {
        method: "POST",
        headers: { "Content-Type": "application/json", ...(API_KEY ? { "X-Api-Key": API_KEY } : {}) },
        body: JSON.stringify({ host: srv.host, port: srv.port, tls: srv.tls, username: srv.username, password: srv.password, index }),
      });
      const data = await res.json();
      setResult(data);
    } catch (e) {
      setResult({ success: false, error: e.message });
    } finally { setTesting(false); }
  };

  const i = index;
  return (
    <div style={{ ...sectionStyle, position: "relative" }}>
      <button onClick={() => removeServer(i)} style={{ position: "absolute", top: 12, right: 12, background: "transparent", border: "none", color: theme.error, cursor: "pointer", fontSize: 16 }}>×</button>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        <ConfigField label="Name" value={srv.name} onChange={v => update(`servers.${i}.name`, v)} />
        <ConfigField label="Host" value={srv.host} onChange={v => update(`servers.${i}.host`, v)} />
        <ConfigField label="Port" value={srv.port} onChange={v => update(`servers.${i}.port`, v)} type="number" />
        <ConfigField label="Connections" value={srv.connections} onChange={v => update(`servers.${i}.connections`, v)} type="number" />
        <ConfigField label="Username" value={srv.username} onChange={v => update(`servers.${i}.username`, v)} />
        <ConfigField label="Password" value={srv.password} onChange={v => update(`servers.${i}.password`, v)} type="password" />
        <ConfigField label="Priority" value={srv.priority} onChange={v => update(`servers.${i}.priority`, v)} type="number" />
        <div style={{ display: "flex", alignItems: "end", paddingBottom: 10 }}>
          <ConfigField label="TLS" value={srv.tls} onChange={v => update(`servers.${i}.tls`, v)} type="checkbox" />
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginTop: 4 }}>
        <button onClick={handleTest} disabled={testing || !srv.host || !srv.port} style={{
          padding: "6px 14px", borderRadius: 8, border: "none",
          background: theme.accentDim, color: theme.accent,
          cursor: testing || !srv.host || !srv.port ? "default" : "pointer",
          fontSize: 11, fontFamily: font, fontWeight: 600, whiteSpace: "nowrap",
          opacity: testing || !srv.host || !srv.port ? 0.5 : 1,
        }}>{testing ? "Testing..." : "Test Connection"}</button>
        {result && (
          <span style={{ fontSize: 11, color: result.success ? (result.warning ? theme.warning : theme.success) : theme.error }}>
            {result.success ? (result.warning || `Connected (${result.elapsed_ms}ms)`) : result.error}
          </span>
        )}
      </div>
    </div>
  );
}

function ConfigEditor() {
  const [cfg, setCfg] = useState(null);
  const [configPath, setConfigPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [section, setSection] = useState("general");

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiFetch("/api/config");
      setCfg(data.config || {});
      setConfigPath(data.path || "");
    } catch (e) { setError(e.message); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { fetchConfig(); }, [fetchConfig]);

  const update = (path, value) => {
    setCfg(prev => {
      const copy = JSON.parse(JSON.stringify(prev));
      const parts = path.split(".");
      let obj = copy;
      for (let i = 0; i < parts.length - 1; i++) {
        const key = isNaN(parts[i]) ? parts[i] : parseInt(parts[i]);
        obj = obj[key];
      }
      obj[parts[parts.length - 1]] = value;
      return copy;
    });
    setSuccess("");
  };

  const handleSave = async () => {
    setSaving(true);
    setError("");
    setSuccess("");
    try {
      const res = await apiFetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ config: cfg }),
      });
      if (res.error) { setError(res.error); }
      else { setSuccess("Config saved and reloaded"); setTimeout(() => setSuccess(""), 3000); }
    } catch (e) { setError(e.message); }
    finally { setSaving(false); }
  };

  const addServer = () => {
    const servers = [...(cfg.servers || []), { name: "", host: "", port: 563, tls: true, username: "", password: "", connections: 10, priority: cfg.servers?.length || 0 }];
    setCfg(prev => ({ ...prev, servers }));
  };

  const removeServer = (i) => {
    setCfg(prev => ({ ...prev, servers: prev.servers.filter((_, idx) => idx !== i) }));
  };

  const addCategory = () => {
    setCfg(prev => ({ ...prev, categories: [...(prev.categories || []), { name: "", dir: "" }] }));
  };

  const removeCategory = (i) => {
    setCfg(prev => ({ ...prev, categories: prev.categories.filter((_, idx) => idx !== i) }));
  };

  if (loading) return <div style={{ padding: 40, textAlign: "center", color: theme.textDim }}>Loading config...</div>;
  if (!cfg) return null;

  const sections = [
    { key: "general", label: "General" },
    { key: "servers", label: "Servers" },
    { key: "categories", label: "Categories" },
    { key: "downloads", label: "Downloads" },
    { key: "postprocess", label: "Post-Processing" },
    { key: "api", label: "API & Auth" },
    { key: "notifications", label: "Notifications" },
  ];

  return (
    <div style={{ padding: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
        <span style={{ fontSize: 11, color: theme.textDim, fontFamily: mono }}>{configPath}</span>
        <div style={{ display: "flex", gap: 8 }}>
          <button onClick={fetchConfig} style={{ padding: "6px 14px", borderRadius: 8, border: `1px solid ${theme.border}`, background: "transparent", color: theme.textMuted, cursor: "pointer", fontSize: 12, fontFamily: font }}>Reload</button>
          <button onClick={handleSave} disabled={saving} style={{ padding: "6px 14px", borderRadius: 8, border: "none", background: theme.accent, color: theme.bg, cursor: "pointer", fontSize: 12, fontWeight: 600, fontFamily: font, opacity: saving ? 0.5 : 1 }}>{saving ? "Saving..." : "Save"}</button>
        </div>
      </div>
      <div style={{ fontSize: 11, color: theme.textDim, marginBottom: 12 }}>Passwords shown as ******** — leaving unchanged preserves originals.</div>
      {error && <div style={{ color: theme.error, fontSize: 12, marginBottom: 8, padding: "8px 12px", background: theme.errorDim, borderRadius: 8 }}>{error}</div>}
      {success && <div style={{ color: theme.success, fontSize: 12, marginBottom: 8, padding: "8px 12px", background: "rgba(16,185,129,0.12)", borderRadius: 8 }}>{success}</div>}

      <div style={{ display: "flex", gap: 6, marginBottom: 16, flexWrap: "wrap" }}>
        {sections.map(s => (
          <button key={s.key} onClick={() => setSection(s.key)} style={{
            padding: "6px 14px", borderRadius: 20, fontSize: 12, fontWeight: 500, cursor: "pointer", fontFamily: font,
            border: section === s.key ? "none" : `1px solid ${theme.border}`,
            background: section === s.key ? theme.accentDim : "transparent",
            color: section === s.key ? theme.accent : theme.textMuted,
          }}>{s.label}</button>
        ))}
      </div>

      {section === "general" && <div style={sectionStyle}>
        <ConfigField label="Download Directory" value={cfg.general?.download_dir} onChange={v => update("general.download_dir", v)} />
        <ConfigField label="Complete Directory" value={cfg.general?.complete_dir} onChange={v => update("general.complete_dir", v)} />
        <ConfigField label="Watch Directory" value={cfg.general?.watch_dir} onChange={v => update("general.watch_dir", v)} />
        <ConfigField label="Log Level" value={cfg.general?.log_level} onChange={v => update("general.log_level", v)} placeholder="debug, info, warn, error" />
      </div>}

      {section === "servers" && <div>
        {(cfg.servers || []).map((srv, i) => (
          <ServerCard key={i} srv={srv} index={i} update={update} removeServer={removeServer} />
        ))}
        <button onClick={addServer} style={{ padding: "8px 16px", borderRadius: 8, border: `1px dashed ${theme.border}`, background: "transparent", color: theme.textMuted, cursor: "pointer", width: "100%", fontSize: 13, fontFamily: font }}>+ Add Server</button>
      </div>}

      {section === "categories" && <div>
        {(cfg.categories || []).map((cat, i) => (
          <div key={i} style={{ ...sectionStyle, display: "flex", gap: 10, alignItems: "end" }}>
            <div style={{ flex: 1 }}><ConfigField label="Name" value={cat.name} onChange={v => update(`categories.${i}.name`, v)} /></div>
            <div style={{ flex: 1 }}><ConfigField label="Directory" value={cat.dir} onChange={v => update(`categories.${i}.dir`, v)} /></div>
            <button onClick={() => removeCategory(i)} style={{ background: "transparent", border: "none", color: theme.error, cursor: "pointer", fontSize: 16, paddingBottom: 14 }}>×</button>
          </div>
        ))}
        <button onClick={addCategory} style={{ padding: "8px 16px", borderRadius: 8, border: `1px dashed ${theme.border}`, background: "transparent", color: theme.textMuted, cursor: "pointer", width: "100%", fontSize: 13, fontFamily: font }}>+ Add Category</button>
      </div>}

      {section === "downloads" && <div style={sectionStyle}>
        <ConfigField label="Max Retries" value={cfg.downloads?.max_retries} onChange={v => update("downloads.max_retries", v)} type="number" />
        <ConfigField label="Speed Limit (KB/s, 0 = unlimited)" value={cfg.downloads?.speed_limit_kbps} onChange={v => update("downloads.speed_limit_kbps", v)} type="number" />
        <ConfigField label="Temp Directory" value={cfg.downloads?.temp_dir} onChange={v => update("downloads.temp_dir", v)} />
      </div>}

      {section === "postprocess" && <div style={sectionStyle}>
        <ConfigField label="PAR2 Verify/Repair" value={cfg.postprocess?.par2_enabled} onChange={v => update("postprocess.par2_enabled", v)} type="checkbox" />
        <ConfigField label="Extract Archives" value={cfg.postprocess?.unpack_enabled} onChange={v => update("postprocess.unpack_enabled", v)} type="checkbox" />
        <ConfigField label="Cleanup After Unpack" value={cfg.postprocess?.cleanup_after_unpack} onChange={v => update("postprocess.cleanup_after_unpack", v)} type="checkbox" />
        <ConfigField label="PAR2 Path" value={cfg.postprocess?.par2_path} onChange={v => update("postprocess.par2_path", v)} />
        <ConfigField label="7z Path" value={cfg.postprocess?.sevenz_path} onChange={v => update("postprocess.sevenz_path", v)} />
      </div>}

      {section === "api" && <div style={sectionStyle}>
        <ConfigField label="Listen Address" value={cfg.api?.listen} onChange={v => update("api.listen", v)} />
        <ConfigField label="Port" value={cfg.api?.port} onChange={v => update("api.port", v)} type="number" />
        <APIKeyManager />
        <div style={{ marginTop: 16, paddingTop: 16, borderTop: `1px solid ${theme.border}` }}>
          <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 12, color: theme.text }}>Forward Auth (Authelia / Pocket ID)</div>
          <ConfigField label="Enabled" value={cfg.api?.forward_auth?.enabled} onChange={v => update("api.forward_auth.enabled", v)} type="checkbox" />
          {cfg.api?.forward_auth?.enabled && <>
            <ConfigField label="User Header" value={cfg.api?.forward_auth?.user_header} onChange={v => update("api.forward_auth.user_header", v)} placeholder="Remote-User" />
            <ConfigField label="Groups Header" value={cfg.api?.forward_auth?.groups_header} onChange={v => update("api.forward_auth.groups_header", v)} placeholder="Remote-Groups" />
          </>}
        </div>
      </div>}

      {section === "notifications" && <div style={sectionStyle}>
        <ConfigField label="Notify on Complete" value={cfg.notifications?.on_complete} onChange={v => update("notifications.on_complete", v)} type="checkbox" />
        <ConfigField label="Notify on Failure" value={cfg.notifications?.on_failure} onChange={v => update("notifications.on_failure", v)} type="checkbox" />
        {(cfg.notifications?.webhooks || []).map((wh, i) => (
          <div key={i} style={{ marginTop: 12, padding: 12, background: theme.surface, borderRadius: 8, border: `1px solid ${theme.border}` }}>
            <ConfigField label="Webhook Name" value={wh.name} onChange={v => update(`notifications.webhooks.${i}.name`, v)} />
            <ConfigField label="URL" value={wh.url} onChange={v => update(`notifications.webhooks.${i}.url`, v)} />
            <ConfigField label="Template (Go template, optional)" value={wh.template} onChange={v => update(`notifications.webhooks.${i}.template`, v)} />
          </div>
        ))}
      </div>}
    </div>
  );
}

function HeaderBtn({ icon, label, onClick, accent }) {
  const [h, setH] = useState(false);
  return (
    <button onClick={onClick} onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)}
      style={{
        display: "flex", alignItems: "center", gap: 6, padding: label ? "7px 14px" : "7px", borderRadius: 8, cursor: "pointer", fontSize: 13, fontWeight: 500,
        border: accent ? "none" : `1px solid ${theme.border}`,
        background: accent ? (h ? theme.accent : theme.accentDim) : (h ? theme.surfaceHover : "transparent"),
        color: accent ? (h ? theme.bg : theme.accent) : theme.textMuted, transition: "all 0.15s ease",
      }}
    >{icon}{label}</button>
  );
}
