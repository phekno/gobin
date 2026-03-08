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

// --- Queue item ---
function QueueItem({ job, onPause, onResume, onRemove }) {
  const [hovered, setHovered] = useState(false);
  return (
    <div
      onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
      style={{
        padding: "16px 20px", background: hovered ? theme.surfaceHover : "transparent",
        borderBottom: `1px solid ${theme.borderSubtle}`, transition: "background 0.15s ease", cursor: "default",
      }}
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
        <div style={{ display: "flex", alignItems: "center", gap: 6, opacity: hovered ? 1 : 0, transition: "opacity 0.15s ease" }}>
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
function HistoryItem({ item }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 16, padding: "12px 20px", borderBottom: `1px solid ${theme.borderSubtle}`, fontSize: 13 }}>
      <div style={{ width: 20, display: "flex", justifyContent: "center" }}>
        {item.status === "completed" ? <span style={{ color: theme.success }}><Icons.Check /></span> : <span style={{ color: theme.error }}><Icons.X /></span>}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: mono, fontSize: 13, color: theme.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{item.name}</div>
        <div style={{ display: "flex", gap: 12, marginTop: 3, fontSize: 11, color: theme.textMuted }}>
          <CategoryTag category={item.category} />
          <span>{formatBytes(item.total_bytes)}</span>
        </div>
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

  // SSE for live queue updates
  useEffect(() => {
    const url = API_KEY ? `/api/events?apikey=${API_KEY}` : "/api/events";
    const es = new EventSource(url);
    es.addEventListener("queue", (e) => {
      try {
        const data = JSON.parse(e.data);
        setQueueData(data.queue || []);
        setQueuePaused(data.paused || false);
      } catch (err) { /* ignore */ }
    });
    return () => es.close();
  }, []);

  const handlePause = async (id) => { await apiPost(`/api/queue/${id}/pause`); fetchQueue(); };
  const handleResume = async (id) => { await apiPost(`/api/queue/${id}/resume`); fetchQueue(); };
  const handleRemove = async (id) => { await apiDelete(`/api/queue/${id}`); fetchQueue(); };
  const handlePauseAll = async () => { await apiPost("/api/queue/pause"); fetchQueue(); };
  const handleResumeAll = async () => { await apiPost("/api/queue/resume"); fetchQueue(); };

  const activeCount = queueData.filter(j => j.status === "downloading").length;
  const remainingBytes = queueData.filter(j => j.status !== "completed").reduce((a, j) => a + ((j.total_bytes || 0) - (j.downloaded_bytes || 0)), 0);

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
        <StatCard label="Status" value={queuePaused ? "Paused" : "Active"} sub={`uptime: ${status.uptime_secs || 0}s`} icon={<Icons.Activity />} accent={queuePaused ? theme.warning : theme.accent} />
        <StatCard label="Queue" value={queueData.length} sub={`${activeCount} downloading`} icon={<Icons.Download />} accent={theme.accent} />
        <StatCard label="Remaining" value={formatBytes(remainingBytes)} icon={<Icons.Clock />} accent={theme.purple} />
        <StatCard label="Version" value={status.version || "—"} icon={<Icons.Cpu />} accent={theme.success} />
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
            {historyData.map((item, i) => (
              <div key={item.id} style={{ animation: `slideIn 0.3s ease ${i * 0.05}s both` }}><HistoryItem item={item} /></div>
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

// --- Config Editor ---
function ConfigEditor() {
  const [yamlText, setYamlText] = useState("");
  const [configPath, setConfigPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiFetch("/api/config");
      setYamlText(data.config_yaml || "");
      setConfigPath(data.path || "");
    } catch (e) { setError(e.message); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { fetchConfig(); }, [fetchConfig]);

  const handleSave = async () => {
    setSaving(true);
    setError("");
    setSuccess("");
    try {
      const res = await apiFetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ config_yaml: yamlText }),
      });
      if (res.error) { setError(res.error); }
      else { setSuccess("Config saved and reloaded"); setTimeout(() => setSuccess(""), 3000); }
    } catch (e) { setError(e.message); }
    finally { setSaving(false); }
  };

  if (loading) return <div style={{ padding: 40, textAlign: "center", color: theme.textDim }}>Loading config...</div>;

  return (
    <div style={{ padding: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
        <div>
          <span style={{ fontSize: 11, color: theme.textDim, fontFamily: mono }}>{configPath}</span>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button onClick={fetchConfig} style={{ padding: "6px 14px", borderRadius: 8, border: `1px solid ${theme.border}`, background: "transparent", color: theme.textMuted, cursor: "pointer", fontSize: 12, fontFamily: font }}>
            Reload
          </button>
          <button onClick={handleSave} disabled={saving} style={{ padding: "6px 14px", borderRadius: 8, border: "none", background: theme.accent, color: theme.bg, cursor: "pointer", fontSize: 12, fontWeight: 600, fontFamily: font, opacity: saving ? 0.5 : 1 }}>
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
      <div style={{ fontSize: 11, color: theme.textDim, marginBottom: 8 }}>
        Passwords and secrets are shown as ********. Leaving them unchanged preserves the original values.
      </div>
      {error && <div style={{ color: theme.error, fontSize: 12, marginBottom: 8, padding: "8px 12px", background: theme.errorDim, borderRadius: 8 }}>{error}</div>}
      {success && <div style={{ color: theme.success, fontSize: 12, marginBottom: 8, padding: "8px 12px", background: "rgba(16,185,129,0.12)", borderRadius: 8 }}>{success}</div>}
      <textarea
        value={yamlText}
        onChange={e => { setYamlText(e.target.value); setSuccess(""); }}
        spellCheck={false}
        style={{
          width: "100%", minHeight: 500, padding: 16, boxSizing: "border-box",
          fontFamily: mono, fontSize: 13, lineHeight: 1.6, tabSize: 2,
          background: theme.bg, color: theme.text,
          border: `1px solid ${theme.border}`, borderRadius: 8,
          resize: "vertical", outline: "none",
        }}
        onFocus={e => e.target.style.borderColor = theme.accent}
        onBlur={e => e.target.style.borderColor = theme.border}
      />
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
