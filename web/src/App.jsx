import { useState, useEffect, useCallback, useRef } from "react";

// --- Theme ---
const theme = {
  bg: "#0e1117",
  surface: "#161b22",
  surfaceHover: "#1c2333",
  surfaceElevated: "#1e2536",
  border: "#27303f",
  borderSubtle: "#1e2536",
  text: "#e2e8f0",
  textMuted: "#7b8ba3",
  textDim: "#4a5568",
  accent: "#22d3ee",
  accentDim: "rgba(34,211,238,0.12)",
  accentGlow: "rgba(34,211,238,0.25)",
  success: "#10b981",
  successDim: "rgba(16,185,129,0.12)",
  warning: "#f59e0b",
  warningDim: "rgba(245,158,11,0.12)",
  error: "#ef4444",
  errorDim: "rgba(239,68,68,0.12)",
  purple: "#a78bfa",
};

const font = `'DM Sans', system-ui, -apple-system, sans-serif`;
const mono = `'JetBrains Mono', 'Fira Code', monospace`;

// --- Mock Data ---
const mockQueue = [
  { id: "j1", name: "The.Matrix.1999.UHD.BluRay.2160p", category: "movies", status: "downloading", progress: 67.3, speed: 48200000, totalBytes: 52428800000, downloadedBytes: 35284070400, segments: { done: 4231, total: 6287, failed: 3 }, addedAt: "2025-03-08T10:15:00Z", eta: "12m 34s" },
  { id: "j2", name: "Better.Call.Saul.S06E13.1080p", category: "tv", status: "downloading", progress: 23.1, speed: 31500000, totalBytes: 4294967296, downloadedBytes: 992137420, segments: { done: 812, total: 3514, failed: 0 }, addedAt: "2025-03-08T11:02:00Z", eta: "1m 45s" },
  { id: "j3", name: "Tool.Fear.Inoculum.FLAC", category: "music", status: "queued", progress: 0, speed: 0, totalBytes: 943718400, downloadedBytes: 0, segments: { done: 0, total: 772, failed: 0 }, addedAt: "2025-03-08T11:15:00Z", eta: "--" },
  { id: "j4", name: "Dune.Part.Two.2024.REMUX", category: "movies", status: "postprocessing", progress: 100, speed: 0, totalBytes: 68719476736, downloadedBytes: 68719476736, segments: { done: 8192, total: 8192, failed: 12 }, addedAt: "2025-03-08T08:00:00Z", eta: "Unpacking..." },
  { id: "j5", name: "Shogun.S01.1080p.COMPLETE", category: "tv", status: "paused", progress: 41.8, speed: 0, totalBytes: 32212254720, downloadedBytes: 13464762473, segments: { done: 2890, total: 6912, failed: 1 }, addedAt: "2025-03-08T09:30:00Z", eta: "Paused" },
];

const mockHistory = [
  { id: "h1", name: "Oppenheimer.2023.UHD.REMUX", category: "movies", status: "completed", totalBytes: 78643200000, completedAt: "2025-03-08T07:22:00Z", duration: "24m 11s" },
  { id: "h2", name: "The.Bear.S03E01.1080p", category: "tv", status: "completed", totalBytes: 3221225472, completedAt: "2025-03-07T22:15:00Z", duration: "1m 38s" },
  { id: "h3", name: "Radiohead.OK.Computer.FLAC", category: "music", status: "completed", totalBytes: 524288000, completedAt: "2025-03-07T20:00:00Z", duration: "32s" },
  { id: "h4", name: "Bad.NZB.File.Test", category: "tv", status: "failed", totalBytes: 1073741824, completedAt: "2025-03-07T18:30:00Z", duration: "5m 12s" },
];

// --- Utilities ---
function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function formatSpeed(bps) {
  if (bps === 0) return "—";
  return formatBytes(bps) + "/s";
}

function timeAgo(dateStr) {
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
  Cpu: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/><line x1="20" y1="9" x2="23" y2="9"/><line x1="20" y1="14" x2="23" y2="14"/><line x1="1" y1="9" x2="4" y2="9"/><line x1="1" y1="14" x2="4" y2="14"/></svg>,
  Activity: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>,
  Folder: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z"/></svg>,
  Upload: () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>,
  Settings: () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06A1.65 1.65 0 0019.32 9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z"/></svg>,
};

// --- Status badge ---
function StatusBadge({ status }) {
  const config = {
    downloading: { color: theme.accent, bg: theme.accentDim, label: "Downloading" },
    queued: { color: theme.textMuted, bg: "rgba(123,139,163,0.1)", label: "Queued" },
    postprocessing: { color: theme.purple, bg: "rgba(167,139,250,0.12)", label: "Processing" },
    paused: { color: theme.warning, bg: theme.warningDim, label: "Paused" },
    completed: { color: theme.success, bg: theme.successDim, label: "Completed" },
    failed: { color: theme.error, bg: theme.errorDim, label: "Failed" },
  }[status] || { color: theme.textMuted, bg: "transparent", label: status };

  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: 5,
      padding: "3px 10px", borderRadius: 20,
      fontSize: 11, fontWeight: 600, letterSpacing: "0.03em",
      color: config.color, background: config.bg,
      textTransform: "uppercase",
    }}>
      {status === "downloading" && <span style={{ width: 6, height: 6, borderRadius: "50%", background: config.color, animation: "pulse 1.5s ease-in-out infinite" }} />}
      {config.label}
    </span>
  );
}

// --- Progress bar ---
function ProgressBar({ progress, status }) {
  const color = status === "paused" ? theme.warning :
                status === "postprocessing" ? theme.purple :
                status === "failed" ? theme.error : theme.accent;
  return (
    <div style={{ width: "100%", height: 4, background: theme.border, borderRadius: 2, overflow: "hidden" }}>
      <div style={{
        width: `${Math.min(progress, 100)}%`, height: "100%",
        background: `linear-gradient(90deg, ${color}, ${color}dd)`,
        borderRadius: 2,
        transition: "width 0.8s cubic-bezier(0.22, 1, 0.36, 1)",
        ...(status === "downloading" ? { boxShadow: `0 0 8px ${color}40` } : {}),
      }} />
    </div>
  );
}

// --- Category tag ---
function CategoryTag({ category }) {
  const colors = {
    movies: "#f472b6", tv: "#60a5fa", music: "#34d399", software: "#fbbf24",
  };
  const c = colors[category] || theme.textMuted;
  return (
    <span style={{
      fontSize: 10, fontWeight: 700, letterSpacing: "0.06em",
      color: c, textTransform: "uppercase", opacity: 0.9,
    }}>
      {category}
    </span>
  );
}

// --- Speed chart (sparkline) ---
function SpeedChart({ data }) {
  const max = Math.max(...data, 1);
  const w = 200, h = 40;
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
  return (
    <div
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        padding: "16px 20px",
        background: hovered ? theme.surfaceHover : "transparent",
        borderBottom: `1px solid ${theme.borderSubtle}`,
        transition: "background 0.15s ease",
        cursor: "default",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 4 }}>
            <CategoryTag category={job.category} />
            <StatusBadge status={job.status} />
          </div>
          <div style={{
            fontSize: 14, fontWeight: 500, color: theme.text,
            fontFamily: mono, whiteSpace: "nowrap",
            overflow: "hidden", textOverflow: "ellipsis",
            maxWidth: 500,
          }}>
            {job.name}
          </div>
        </div>
        <div style={{
          display: "flex", alignItems: "center", gap: 6,
          opacity: hovered ? 1 : 0, transition: "opacity 0.15s ease",
        }}>
          {job.status === "paused" ? (
            <ActionBtn icon={<Icons.Play />} title="Resume" onClick={() => onResume(job.id)} />
          ) : job.status === "downloading" || job.status === "queued" ? (
            <ActionBtn icon={<Icons.Pause />} title="Pause" onClick={() => onPause(job.id)} />
          ) : null}
          <ActionBtn icon={<Icons.Trash />} title="Remove" onClick={() => onRemove(job.id)} color={theme.error} />
        </div>
      </div>

      <ProgressBar progress={job.progress} status={job.status} />

      <div style={{
        display: "flex", alignItems: "center", gap: 16,
        marginTop: 8, fontSize: 12, color: theme.textMuted, fontFamily: mono,
      }}>
        <span>{job.progress.toFixed(1)}%</span>
        <span>{formatBytes(job.downloadedBytes)} / {formatBytes(job.totalBytes)}</span>
        {job.speed > 0 && <span style={{ color: theme.accent }}>{formatSpeed(job.speed)}</span>}
        <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
          <Icons.Clock /> {job.eta}
        </span>
        <span style={{ marginLeft: "auto", fontSize: 11 }}>
          {job.segments.done}/{job.segments.total} segments
          {job.segments.failed > 0 && <span style={{ color: theme.error, marginLeft: 4 }}>({job.segments.failed} failed)</span>}
        </span>
      </div>
    </div>
  );
}

function ActionBtn({ icon, title, onClick, color }) {
  const [h, setH] = useState(false);
  return (
    <button
      title={title} onClick={onClick}
      onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)}
      style={{
        display: "flex", alignItems: "center", justifyContent: "center",
        width: 32, height: 32, borderRadius: 8,
        border: `1px solid ${h ? (color || theme.accent) : theme.border}`,
        background: h ? (color || theme.accent) + "18" : "transparent",
        color: h ? (color || theme.accent) : theme.textMuted,
        cursor: "pointer", transition: "all 0.15s ease",
      }}
    >{icon}</button>
  );
}

// --- History item ---
function HistoryItem({ item }) {
  return (
    <div style={{
      display: "flex", alignItems: "center", gap: 16,
      padding: "12px 20px",
      borderBottom: `1px solid ${theme.borderSubtle}`,
      fontSize: 13,
    }}>
      <div style={{ width: 20, display: "flex", justifyContent: "center" }}>
        {item.status === "completed"
          ? <span style={{ color: theme.success }}><Icons.Check /></span>
          : <span style={{ color: theme.error }}><Icons.X /></span>}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: mono, fontSize: 13, color: theme.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {item.name}
        </div>
        <div style={{ display: "flex", gap: 12, marginTop: 3, fontSize: 11, color: theme.textMuted }}>
          <CategoryTag category={item.category} />
          <span>{formatBytes(item.totalBytes)}</span>
          <span>{item.duration}</span>
        </div>
      </div>
      <span style={{ fontSize: 11, color: theme.textDim, whiteSpace: "nowrap" }}>
        {timeAgo(item.completedAt)}
      </span>
    </div>
  );
}

// --- Stat card ---
function StatCard({ label, value, sub, icon, accent }) {
  return (
    <div style={{
      background: theme.surface,
      border: `1px solid ${theme.border}`,
      borderRadius: 12, padding: "16px 20px",
      display: "flex", flexDirection: "column", gap: 4,
      position: "relative", overflow: "hidden",
    }}>
      <div style={{
        position: "absolute", top: -8, right: -8, width: 48, height: 48,
        borderRadius: "50%", background: (accent || theme.accent) + "0a",
        display: "flex", alignItems: "center", justifyContent: "center",
        color: accent || theme.accent, opacity: 0.5,
      }}>{icon}</div>
      <span style={{ fontSize: 11, color: theme.textMuted, textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 600 }}>{label}</span>
      <span style={{ fontSize: 24, fontWeight: 700, color: theme.text, fontFamily: mono, letterSpacing: "-0.02em" }}>{value}</span>
      {sub && <span style={{ fontSize: 11, color: theme.textDim }}>{sub}</span>}
    </div>
  );
}

// --- Main App ---
export default function GoBinUI() {
  const [tab, setTab] = useState("queue");
  const [speedData, setSpeedData] = useState(() => Array.from({ length: 30 }, () => Math.random() * 50000000));

  // Simulate speed updates
  useEffect(() => {
    const interval = setInterval(() => {
      setSpeedData(prev => {
        const next = [...prev.slice(1), prev[prev.length - 1] + (Math.random() - 0.48) * 8000000];
        return next.map(v => Math.max(0, v));
      });
    }, 1000);
    return () => clearInterval(interval);
  }, []);

  const totalSpeed = mockQueue.reduce((a, j) => a + j.speed, 0);
  const activeCount = mockQueue.filter(j => j.status === "downloading").length;
  const queuedBytes = mockQueue.filter(j => j.status !== "completed").reduce((a, j) => a + (j.totalBytes - j.downloadedBytes), 0);

  return (
    <div style={{
      minHeight: "100vh", background: theme.bg, color: theme.text,
      fontFamily: font, lineHeight: 1.5,
    }}>
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

      {/* --- Top bar --- */}
      <header style={{
        display: "flex", alignItems: "center", justifyContent: "space-between",
        padding: "0 28px", height: 56,
        borderBottom: `1px solid ${theme.border}`,
        background: theme.surface,
        position: "sticky", top: 0, zIndex: 100,
        backdropFilter: "blur(12px)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{
            width: 32, height: 32, borderRadius: 8,
            background: `linear-gradient(135deg, ${theme.accent}, ${theme.purple})`,
            display: "flex", alignItems: "center", justifyContent: "center",
            boxShadow: `0 2px 12px ${theme.accentGlow}`,
          }}>
            <Icons.Download />
          </div>
          <span style={{ fontSize: 18, fontWeight: 700, letterSpacing: "-0.02em" }}>
            Go<span style={{ color: theme.accent }}>Bin</span>
          </span>
          <span style={{ fontSize: 10, color: theme.textDim, fontFamily: mono, marginLeft: 4 }}>v0.1.0</span>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <SpeedChart data={speedData} />
          <div style={{ textAlign: "right", marginLeft: 8 }}>
            <div style={{ fontSize: 16, fontWeight: 700, fontFamily: mono, color: theme.accent }}>
              {formatSpeed(totalSpeed)}
            </div>
            <div style={{ fontSize: 10, color: theme.textDim }}>
              {activeCount} active · {formatBytes(queuedBytes)} left
            </div>
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
          <HeaderBtn icon={<Icons.Upload />} label="Add NZB" accent />
          <HeaderBtn icon={<Icons.Settings />} />
        </div>
      </header>

      {/* --- Tab bar --- */}
      <nav style={{
        display: "flex", gap: 0,
        borderBottom: `1px solid ${theme.border}`,
        background: theme.surface,
        padding: "0 28px",
      }}>
        {[
          { key: "queue", label: "Queue", count: mockQueue.length },
          { key: "history", label: "History", count: mockHistory.length },
        ].map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: "12px 20px", fontSize: 13, fontWeight: 600,
              color: tab === t.key ? theme.text : theme.textMuted,
              background: "none", border: "none", cursor: "pointer",
              borderBottom: `2px solid ${tab === t.key ? theme.accent : "transparent"}`,
              transition: "all 0.15s ease",
              display: "flex", alignItems: "center", gap: 8,
            }}
          >
            {t.label}
            <span style={{
              fontSize: 10, fontWeight: 700, fontFamily: mono,
              padding: "1px 7px", borderRadius: 10,
              background: tab === t.key ? theme.accentDim : theme.borderSubtle,
              color: tab === t.key ? theme.accent : theme.textDim,
            }}>{t.count}</span>
          </button>
        ))}
      </nav>

      {/* --- Stats row --- */}
      <div style={{
        display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12,
        padding: "20px 28px",
      }}>
        <StatCard label="Speed" value={formatSpeed(totalSpeed)} sub="across 2 jobs" icon={<Icons.Activity />} />
        <StatCard label="Active" value={activeCount} sub={`${mockQueue.length} total in queue`} icon={<Icons.Download />} accent={theme.accent} />
        <StatCard label="Remaining" value={formatBytes(queuedBytes)} sub="estimated 14m 19s" icon={<Icons.Clock />} accent={theme.purple} />
        <StatCard label="Connections" value="28 / 28" sub="primary: 20, backup: 8" icon={<Icons.Cpu />} accent={theme.success} />
      </div>

      {/* --- Content --- */}
      <div style={{ padding: "0 28px 28px" }}>
        <div style={{
          background: theme.surface,
          border: `1px solid ${theme.border}`,
          borderRadius: 12,
          overflow: "hidden",
        }}>
          {tab === "queue" && (
            <>
              {mockQueue.map((job, i) => (
                <div key={job.id} style={{ animation: `slideIn 0.3s ease ${i * 0.05}s both` }}>
                  <QueueItem
                    job={job}
                    onPause={(id) => console.log("pause", id)}
                    onResume={(id) => console.log("resume", id)}
                    onRemove={(id) => console.log("remove", id)}
                  />
                </div>
              ))}
              {mockQueue.length === 0 && (
                <div style={{ padding: 60, textAlign: "center", color: theme.textDim }}>
                  <div style={{ fontSize: 14, marginBottom: 4 }}>Queue is empty</div>
                  <div style={{ fontSize: 12 }}>Drop an NZB file or click Add NZB to get started</div>
                </div>
              )}
            </>
          )}
          {tab === "history" && (
            <>
              {mockHistory.map((item, i) => (
                <div key={item.id} style={{ animation: `slideIn 0.3s ease ${i * 0.05}s both` }}>
                  <HistoryItem item={item} />
                </div>
              ))}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function HeaderBtn({ icon, label, accent }) {
  const [h, setH] = useState(false);
  return (
    <button
      onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)}
      style={{
        display: "flex", alignItems: "center", gap: 6,
        padding: label ? "7px 14px" : "7px",
        borderRadius: 8, cursor: "pointer",
        fontSize: 13, fontWeight: 500,
        border: accent ? "none" : `1px solid ${theme.border}`,
        background: accent
          ? h ? theme.accent : theme.accentDim
          : h ? theme.surfaceHover : "transparent",
        color: accent
          ? h ? theme.bg : theme.accent
          : theme.textMuted,
        transition: "all 0.15s ease",
      }}
    >
      {icon}{label}
    </button>
  );
}
