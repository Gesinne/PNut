import { useState, useEffect, useRef } from "react";
import {
  Zap, Clock, Activity, Wifi, WifiOff, AlertTriangle,
  Shield, Radio, BatteryCharging, TrendingUp, Sun, Moon, Gauge,
} from "lucide-react";

const API_URL      = "http://192.168.0.144:5000/api/ups";
const POLL_INTERVAL = 1000;
const HISTORY_MAX   = 50;

// ─── Smooth animated number ───────────────────────────────────────────────────
function useAnimatedValue(target, duration = 700) {
  const [display, setDisplay] = useState(target);
  const s = useRef({ from: target, to: target, t0: null, raf: null });
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      setDisplay(target);
      return;
    }
    const r = s.current;
    if (r.raf) cancelAnimationFrame(r.raf);
    r.from = display; r.to = target; r.t0 = null;
    const tick = (now) => {
      if (!r.t0) r.t0 = now;
      const p = Math.min((now - r.t0) / duration, 1);
      const e = 1 - Math.pow(1 - p, 3);
      setDisplay(Math.round(r.from + (r.to - r.from) * e));
      if (p < 1) r.raf = requestAnimationFrame(tick);
    };
    r.raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(r.raf);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);
  return display;
}

// ─── Theme tokens ─────────────────────────────────────────────────────────────
const T = {
  dark: {
    name:          "dark",
    bgBase:        "#020617",
    bgGrad:        "radial-gradient(ellipse 110% 60% at 50% -10%, #0d1b3e 0%, #020617 60%)",
    card:          "rgba(12,20,40,0.75)",
    card2:         "rgba(20,30,55,0.6)",
    cardWarn:      "rgba(220,38,38,0.08)",
    glass:         "blur(20px)",
    border:        "rgba(255,255,255,0.07)",
    borderStrong:  "rgba(255,255,255,0.12)",
    borderWarn:    "rgba(220,38,38,0.3)",
    text:          "#f1f5f9",
    text2:         "#94a3b8",
    text3:         "#475569",
    shadow:        "none",
    shadowCard:    "0 4px 24px rgba(0,0,0,0.4)",
    barTrack:      "rgba(255,255,255,0.07)",
    headerBg:      "rgba(2,6,23,0.8)",
    headerBorder:  "rgba(255,255,255,0.06)",
    badgeMock:     "rgba(217,119,6,0.15)",
    ringTrack:     "rgba(255,255,255,0.06)",
    accent:        "#6366f1",
  },
  light: {
    name:          "light",
    bgBase:        "#f1f5f9",
    bgGrad:        "linear-gradient(145deg, #f8fafc 0%, #eef2f7 50%, #e8edf5 100%)",
    card:          "#ffffff",
    card2:         "#f8fafc",
    cardWarn:      "#fff5f5",
    glass:         "none",
    border:        "#e2e8f0",
    borderStrong:  "#cbd5e1",
    borderWarn:    "#fecaca",
    text:          "#0f172a",
    text2:         "#475569",
    text3:         "#94a3b8",
    shadow:        "0 1px 3px rgba(0,0,0,0.06)",
    shadowCard:    "0 2px 12px rgba(0,0,0,0.07), 0 1px 3px rgba(0,0,0,0.05)",
    barTrack:      "#e2e8f0",
    headerBg:      "rgba(255,255,255,0.85)",
    headerBorder:  "#e2e8f0",
    badgeMock:     "#fef9c3",
    ringTrack:     "#e2e8f0",
    accent:        "#1e40af",
  },
};

// ─── Mock data ────────────────────────────────────────────────────────────────
let mockTick = 0;
const getMockData = () => {
  mockTick += 0.05;
  const battery = Math.round(Math.max(5, Math.min(100,
    72 + Math.sin(mockTick * 0.3) * 18 + Math.sin(mockTick * 1.1) * 4)));
  const ob   = Math.sin(mockTick * 0.15) > 0.92;
  const lb   = ob && battery < 25;
  const chrg = !ob && battery < 98;
  return {
    battery_charge:    battery,
    battery_voltage:   parseFloat((ob ? 12.1 + (battery / 100) * 1.4 : 13.5 + Math.sin(mockTick * 0.9) * 0.15).toFixed(2)),
    input_voltage:     parseFloat((230.4 + Math.sin(mockTick * 0.7) * 3.2).toFixed(1)),
    input_frequency:   parseFloat((50.0 + Math.sin(mockTick * 0.4) * 0.15).toFixed(2)),
    ups_load:          Math.round(Math.max(5, Math.min(90, 34 + Math.sin(mockTick * 0.5) * 12 + Math.random() * 4))),
    battery_runtime:   Math.round((battery / 100) * 55 + Math.sin(mockTick * 0.4) * 3),
    ups_realpower_nom: 490,
    ups_status:        ob ? (lb ? "OB LB DISCHRG" : "OB DISCHRG") : (chrg ? "OL CHRG" : "OL"),
    battery_charge_low: 10,
    ups_temperature:   null,
  };
};

// ─── NUT flags ────────────────────────────────────────────────────────────────
const FLAG_META = {
  OL:      { label: "Online",       color: "#16a34a" },
  OB:      { label: "En batería",   color: "#dc2626" },
  LB:      { label: "Bat. baja",    color: "#dc2626" },
  RB:      { label: "Cambiar bat.", color: "#ea580c" },
  CHRG:    { label: "Cargando",     color: "#3b82f6" },
  DISCHRG: { label: "Descargando",  color: "#d97706" },
  BYPASS:  { label: "Bypass",       color: "#7c3aed" },
  OVER:    { label: "Sobrecarga",   color: "#dc2626" },
  TRIM:    { label: "Rec. voltaje", color: "#d97706" },
  BOOST:   { label: "Ele. voltaje", color: "#d97706" },
  OFF:     { label: "Apagado",      color: "#6b7280" },
  CAL:     { label: "Calibrando",   color: "#6b7280" },
};
const parseFlags = (s = "") =>
  s.trim().split(/\s+/).filter(f => f in FLAG_META).map(f => ({ key: f, ...FLAG_META[f] }));
const hasFlag = (s, f) => (s ?? "").includes(f);

const getMode = (s) => {
  if (hasFlag(s, "OVER"))    return { label: "SOBRECARGA",  color: "#dc2626" };
  if (hasFlag(s, "BYPASS"))  return { label: "BYPASS",      color: "#7c3aed" };
  if (hasFlag(s, "TRIM"))    return { label: "RECORTANDO V", color: "#d97706" };
  if (hasFlag(s, "BOOST"))   return { label: "ELEVANDO V",  color: "#d97706" };
  if (hasFlag(s, "LB"))      return { label: "BAT. BAJA",   color: "#dc2626" };
  if (hasFlag(s, "DISCHRG")) return { label: "DESCARGANDO", color: "#d97706" };
  if (hasFlag(s, "OB"))      return { label: "EN BATERÍA",  color: "#dc2626" };
  if (hasFlag(s, "CHRG"))    return { label: "CARGANDO",    color: "#3b82f6" };
  if (hasFlag(s, "OL"))      return { label: "FLOTACIÓN",   color: "#16a34a" };
  return { label: "DESCONOCIDO", color: "#6b7280" };
};

// ─── Battery voltage health (12V lead-acid) ───────────────────────────────────
const batteryHealth = (voltage, statusStr) => {
  if (voltage == null || Number.isNaN(voltage)) return { level: "ok", label: null };
  const ob = hasFlag(statusStr, "OB");
  const ol = hasFlag(statusStr, "OL") && !ob;
  if (ol) {
    if (voltage < 12.4) return { level: "critical", label: "BATERÍA DEGRADADA" };
    if (voltage < 13.0) return { level: "low",      label: "TENSIÓN BAJA" };
  }
  if (ob) {
    if (voltage < 10.8) return { level: "critical", label: "BATERÍA DEGRADADA" };
    if (voltage < 11.5) return { level: "low",      label: "TENSIÓN BAJA" };
  }
  return { level: "ok", label: null };
};

const battColor  = (p) => p > 50 ? "#16a34a" : p > 20 ? "#d97706" : "#dc2626";
const loadColor  = (p) => p > 80 ? "#dc2626" : p > 60 ? "#d97706" : "#16a34a";

// ─── Sparkline ────────────────────────────────────────────────────────────────
function Sparkline({ data, color }) {
  if (data.length < 2) return (
    <div style={{ flex: 1, height: 50, display: "flex", alignItems: "center", justifyContent: "center" }}>
      <span style={{ fontSize: "0.6rem", color: "#64748b", letterSpacing: "0.1em" }}>Acumulando…</span>
    </div>
  );
  const W = 100, H = 100, PAD = 4;
  const xStart = PAD, xEnd = W - PAD;
  const span = xEnd - xStart;
  const pts = data.map((v, i) =>
    `${(xStart + (i / (data.length - 1)) * span).toFixed(1)},${(H - (v / 100) * H).toFixed(1)}`).join(" ");
  const lx = xEnd, ly = H - (data[data.length - 1] / 100) * H;
  return (
    <div style={{ flex: 1, height: 50, paddingTop: 4, paddingBottom: 4, overflow: "hidden" }}>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none"
        style={{ width: "100%", height: "100%", overflow: "hidden", display: "block" }}>
        <defs>
          <linearGradient id="sfill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.22" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <polyline points={`${xStart},${H} ${pts} ${xEnd},${H}`} fill="url(#sfill)" stroke="none" />
        <polyline points={pts} fill="none" stroke={color} strokeWidth="2.5"
          strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke"
          style={{ filter: `drop-shadow(0 0 4px ${color}88)` }} />
        <circle cx={lx} cy={ly} r="3.5" fill={color} vectorEffect="non-scaling-stroke" />
      </svg>
    </div>
  );
}

// ─── Battery ring ─────────────────────────────────────────────────────────────
const RS = 200, STR = 12, RAD = (RS - STR) / 2, CIRC = 2 * Math.PI * RAD;

function BatteryRing({ pct, displayPct, mode, th }) {
  const color  = battColor(pct);
  const offset = CIRC - (pct / 100) * CIRC;
  const isDark = th.name === "dark";
  return (
    <div style={{ position: "relative", width: RS, height: RS, flexShrink: 0 }}>
      {/* Outer glow (dark only) */}
      {isDark && (
        <div style={{
          position: "absolute", inset: -16,
          borderRadius: "50%",
          background: `radial-gradient(circle, ${color}18 0%, transparent 70%)`,
          pointerEvents: "none",
          transition: "background 0.6s ease",
        }} />
      )}
      <svg width={RS} height={RS} style={{ transform: "rotate(-90deg)", position: "relative", zIndex: 1 }}>
        {/* Track */}
        <circle cx={RS/2} cy={RS/2} r={RAD} fill="none"
          stroke={th.ringTrack} strokeWidth={STR} />
        {/* Progress */}
        <circle cx={RS/2} cy={RS/2} r={RAD} fill="none"
          stroke={color} strokeWidth={STR} strokeLinecap="round"
          strokeDasharray={CIRC} strokeDashoffset={offset}
          style={{
            transition: "stroke-dashoffset 0.8s cubic-bezier(0.4,0,0.2,1), stroke 0.5s ease",
            filter: isDark ? `drop-shadow(0 0 8px ${color}bb)` : "none",
          }} />
      </svg>
      <div style={{
        position: "absolute", inset: 0, zIndex: 2,
        display: "flex", flexDirection: "column",
        alignItems: "center", justifyContent: "center",
        gap: 4,
      }}>
        <span style={{
          fontFamily: "'Fira Code', monospace",
          fontSize: "3.2rem", fontWeight: 700,
          color, lineHeight: 1,
          fontFeatureSettings: '"tnum"',
          textShadow: isDark ? `0 0 32px ${color}55` : "none",
          transition: "color 0.5s ease",
        }}>{displayPct}</span>
        <span style={{
          fontFamily: "'Fira Sans', sans-serif",
          fontSize: "0.6rem", fontWeight: 600,
          color: mode.color, letterSpacing: "0.2em",
          textTransform: "uppercase",
          transition: "color 0.4s ease",
        }}>{mode.label}</span>
      </div>
    </div>
  );
}

// ─── Stat card ────────────────────────────────────────────────────────────────
function StatCard({ icon: Icon, label, value, unit, accent = "#3b82f6", warn, th }) {
  return (
    <div style={{
      background: warn ? th.cardWarn : th.card,
      border: `1px solid ${warn ? th.borderWarn : th.border}`,
      borderRadius: 14,
      padding: "14px 16px",
      boxShadow: th.shadowCard,
      backdropFilter: th.glass,
      transition: "all 0.3s ease",
      display: "flex", flexDirection: "column", gap: 10,
    }}>
      {/* Icon pill + label */}
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <div style={{
          width: 28, height: 28, borderRadius: 8, flexShrink: 0,
          background: `${accent}20`,
          border: `1px solid ${accent}30`,
          display: "flex", alignItems: "center", justifyContent: "center",
        }}>
          <Icon size={13} color={accent} strokeWidth={2.2} />
        </div>
        <span style={{
          fontFamily: "'Fira Sans', sans-serif",
          fontSize: "0.6rem", color: th.text3,
          letterSpacing: "0.14em", textTransform: "uppercase",
          fontWeight: 500,
        }}>{label}</span>
      </div>
      {/* Value */}
      <div style={{ display: "flex", alignItems: "baseline", gap: 3, paddingLeft: 2 }}>
        <span style={{
          fontFamily: "'Fira Code', monospace",
          fontSize: "1.85rem", fontWeight: 700,
          color: warn ? "#dc2626" : th.text,
          fontFeatureSettings: '"tnum"',
          lineHeight: 1,
          transition: "color 0.3s ease",
          overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
          maxWidth: "100%",
        }}>{value ?? "—"}</span>
        {unit && (
          <span style={{
            fontFamily: "'Fira Code', monospace",
            fontSize: "0.8rem", color: th.text3, fontWeight: 500,
          }}>{unit}</span>
        )}
      </div>
    </div>
  );
}

// ─── Power consumption bar ────────────────────────────────────────────────────
function PowerBar({ loadPct, nominalW, hasData, th }) {
  const currentW = Math.round((loadPct / 100) * nominalW);
  const pct      = Math.min(loadPct, 100);
  const color    = hasData ? loadColor(loadPct) : th.text3;
  const isDark   = th.name === "dark";
  return (
    <div style={{
      background: th.card, border: `1px solid ${th.border}`,
      borderRadius: 14, padding: "14px 16px",
      boxShadow: th.shadowCard, backdropFilter: th.glass,
    }}>
      {/* Header row */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <div style={{
            width: 28, height: 28, borderRadius: 8,
            background: "#7c3aed20", border: "1px solid #7c3aed30",
            display: "flex", alignItems: "center", justifyContent: "center",
          }}>
            <Zap size={13} color="#7c3aed" strokeWidth={2.2} />
          </div>
          <span style={{ fontSize: "0.6rem", color: th.text3, letterSpacing: "0.14em", textTransform: "uppercase", fontWeight: 500 }}>
            Consumo SAI
          </span>
        </div>
        <span style={{ fontSize: "0.6rem", color: th.text3, fontFamily: "'Fira Code', monospace" }}>
          / {nominalW} W
        </span>
      </div>

      {/* Watt value */}
      <div style={{ display: "flex", alignItems: "baseline", gap: 6, marginBottom: 10, paddingLeft: 2 }}>
        <span style={{
          fontFamily: "'Fira Code', monospace",
          fontSize: "1.85rem", fontWeight: 700, color,
          fontFeatureSettings: '"tnum"',
          lineHeight: 1, transition: "color 0.4s ease",
        }}>{hasData ? currentW : "—"}</span>
        <span style={{ fontFamily: "'Fira Code', monospace", fontSize: "0.8rem", color: th.text3, fontWeight: 500 }}>W</span>
        {!hasData && (
          <span style={{
            fontSize: "0.6rem", color: th.text3, fontStyle: "italic",
            marginLeft: 4, alignSelf: "center",
          }}>
            (UPS no reporta carga)
          </span>
        )}
      </div>

      {/* Bar */}
      <div style={{ height: 7, borderRadius: 999, background: th.barTrack, overflow: "hidden" }}>
        <div style={{
          height: "100%",
          width: hasData ? `${pct}%` : "0%",
          background: loadPct > 80
            ? `linear-gradient(90deg, #f97316, #dc2626)`
            : loadPct > 60
            ? `linear-gradient(90deg, #eab308, ${color})`
            : `linear-gradient(90deg, ${color}cc, ${color})`,
          borderRadius: 999,
          transition: "width 0.8s cubic-bezier(0.4,0,0.2,1), background 0.4s ease",
          boxShadow: isDark && hasData ? `0 0 8px ${color}88` : "none",
        }} />
      </div>

      {/* Label row */}
      <div style={{ display: "flex", justifyContent: "space-between", marginTop: 7, alignItems: "center" }}>
        <span style={{
          fontFamily: "'Fira Code', monospace",
          fontSize: "0.65rem", fontWeight: 600, color,
        }}>{hasData ? `${pct}%` : "—"}</span>
        <span style={{
          fontSize: "0.62rem", letterSpacing: "0.1em",
          fontWeight: loadPct > 80 ? 600 : 400,
          color: !hasData
            ? th.text3
            : loadPct > 80 ? "#dc2626"
            : loadPct > 60 ? "#d97706"
            : th.text3,
        }}>
          {!hasData ? "SIN DATOS DE CARGA"
            : loadPct > 80 ? "⚠ CERCA DEL LÍMITE"
            : loadPct > 60 ? "CARGA ALTA"
            : "CARGA NORMAL"}
        </span>
      </div>
    </div>
  );
}

// ─── Flag badge ───────────────────────────────────────────────────────────────
function FlagBadge({ label, color, th }) {
  return (
    <span style={{
      fontFamily: "'Fira Code', monospace",
      fontSize: "0.6rem", fontWeight: 600,
      letterSpacing: "0.07em", color,
      background: `${color}18`,
      border: `1px solid ${color}35`,
      borderRadius: 20,
      padding: "3px 10px",
      whiteSpace: "nowrap",
      boxShadow: th.name === "dark" ? `0 0 8px ${color}22` : "none",
    }}>{label}</span>
  );
}

// ─── Card wrapper ─────────────────────────────────────────────────────────────
const Card = ({ children, th, warn, style: s = {} }) => (
  <div style={{
    background: warn ? th.cardWarn : th.card,
    border: `1px solid ${warn ? th.borderWarn : th.border}`,
    borderRadius: 14, padding: "16px",
    boxShadow: th.shadowCard,
    backdropFilter: th.glass,
    transition: "all 0.3s ease",
    ...s,
  }}>{children}</div>
);

const SLabel = ({ children, th }) => (
  <div style={{
    fontSize: "0.62rem", color: th.text3,
    letterSpacing: "0.16em", textTransform: "uppercase",
    fontWeight: 500, marginBottom: 8,
  }}>{children}</div>
);

// ─── Alert pill helper ────────────────────────────────────────────────────────
const alertPill = (color) => ({
  display: "flex", alignItems: "center", gap: 5,
  background: `${color}18`, border: `1px solid ${color}40`,
  borderRadius: 20, padding: "3px 9px",
  fontSize: "0.6rem", color, fontWeight: 700, letterSpacing: "0.07em",
  whiteSpace: "nowrap",
});

// ─── Main ─────────────────────────────────────────────────────────────────────
export default function UPSMonitor() {
  const init = () => {
    const s = localStorage.getItem("ups-theme");
    return s || (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  };

  const [themeName, setThemeName] = useState(init);
  const [data,    setData]    = useState(getMockData());
  const [isMock,  setIsMock]  = useState(true);
  const [ts,      setTs]      = useState(new Date());
  const [history, setHistory] = useState([]);
  const [wide,    setWide]    = useState(window.innerWidth >= 800);
  const lastLoad    = useRef(0);
  const lastNominal = useRef(490);
  const sawLoad     = useRef(false);

  const th = T[themeName];

  const toggleTheme = () => {
    const n = themeName === "dark" ? "light" : "dark";
    setThemeName(n);
    localStorage.setItem("ups-theme", n);
  };

  useEffect(() => {
    document.body.style.background = T[themeName].bgBase;
  }, [themeName]);

  useEffect(() => {
    const h = () => setWide(window.innerWidth >= 800);
    window.addEventListener("resize", h);
    return () => window.removeEventListener("resize", h);
  }, []);

  useEffect(() => {
    const poll = async () => {
      let next;
      try {
        const r = await fetch(API_URL, { signal: AbortSignal.timeout(900) });
        if (!r.ok) throw new Error();
        next = await r.json();
        if (next.ups_load    != null) lastLoad.current    = next.ups_load;
        if (next.ups_load    != null) sawLoad.current = true;
        if (next.ups_realpower_nom != null) lastNominal.current = next.ups_realpower_nom;
        setIsMock(false);
      } catch {
        next = getMockData();
        setIsMock(true);
      }
      setData(next);
      setHistory(p => [...p.slice(-(HISTORY_MAX - 1)), next.battery_charge ?? 0]);
      setTs(new Date());
    };
    poll();
    const id = setInterval(poll, POLL_INTERVAL);
    return () => clearInterval(id);
  }, []);

  const statusStr  = data.ups_status ?? "OL";
  const onBattery  = hasFlag(statusStr, "OB");
  const lowBat     = hasFlag(statusStr, "LB");
  const replaceBat = hasFlag(statusStr, "RB");
  const overload   = hasFlag(statusStr, "OVER");
  const flags      = parseFlags(statusStr);
  const battPct    = data.battery_charge ?? 0;
  const bColor     = battColor(battPct);
  const mode       = getMode(statusStr);
  const loadPct    = data.ups_load ?? lastLoad.current;
  const nominalW   = data.ups_realpower_nom ?? lastNominal.current;
  const hasLoadData = isMock || sawLoad.current || data.ups_load != null;
  const battHealth = isMock ? { level: "ok", label: null } : batteryHealth(data.battery_voltage, statusStr);
  const displayPct = useAnimatedValue(battPct);
  const histMax    = history.length ? Math.max(...history) : 100;
  const histMin    = history.length ? Math.min(...history) : 0;
  const isDark     = themeName === "dark";

  return (
    <>
      <style>{`
        @import url('https://fonts.googleapis.com/css2?family=Fira+Code:wght@400;500;600;700&family=Fira+Sans:wght@300;400;500;600;700&display=swap');
        *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
        html, body, #root { height: 100%; width: 100%; }
        body { overflow-x: hidden; }

        @keyframes blink  { 0%,100%{opacity:1} 50%{opacity:0.2} }
        @keyframes blob1  { 0%,100%{transform:translate(0,0) scale(1)} 50%{transform:translate(40px,-30px) scale(1.15)} }
        @keyframes blob2  { 0%,100%{transform:translate(0,0) scale(1)} 50%{transform:translate(-35px,25px) scale(0.9)} }
        @keyframes blob3  { 0%,100%{transform:translate(0,0) scale(1)} 50%{transform:translate(20px,35px) scale(1.05)} }
        @keyframes alertIn{ from{opacity:0;transform:translateY(-5px)} to{opacity:1;transform:translateY(0)} }

        .dot-live  { animation: blink 2s ease-in-out infinite; }
        .alert-new { animation: alertIn 0.3s ease-out; }

        .theme-btn {
          cursor: pointer; background: none; border: none; padding: 8px;
          border-radius: 10px; display: flex; align-items: center;
          transition: background 0.2s ease;
        }
        .theme-btn:hover { background: rgba(128,128,128,0.14); }

        @media (prefers-reduced-motion: reduce) {
          .dot-live, .blob { animation: none !important; }
        }
      `}</style>

      {/* ── Ambient blobs (dark only) ── */}
      {isDark && (
        <>
          <div className="blob" style={{
            position: "fixed", top: "-10%", left: "15%",
            width: 420, height: 420, borderRadius: "50%",
            background: "radial-gradient(circle, #6366f122 0%, transparent 70%)",
            filter: "blur(50px)", pointerEvents: "none", zIndex: 0,
            animation: "blob1 18s ease-in-out infinite",
          }} />
          <div className="blob" style={{
            position: "fixed", top: "40%", right: "-5%",
            width: 360, height: 360, borderRadius: "50%",
            background: "radial-gradient(circle, #3b82f618 0%, transparent 70%)",
            filter: "blur(60px)", pointerEvents: "none", zIndex: 0,
            animation: "blob2 22s ease-in-out infinite",
          }} />
          <div className="blob" style={{
            position: "fixed", bottom: "-5%", left: "40%",
            width: 300, height: 300, borderRadius: "50%",
            background: `radial-gradient(circle, ${bColor}14 0%, transparent 70%)`,
            filter: "blur(50px)", pointerEvents: "none", zIndex: 0,
            animation: "blob3 16s ease-in-out infinite",
            transition: "background 1s ease",
          }} />
        </>
      )}

      <div style={{
        minHeight: "100dvh",
        background: th.bgGrad,
        fontFamily: "'Fira Sans', sans-serif",
        color: th.text,
        display: "flex", flexDirection: "column",
        transition: "background 0.4s ease, color 0.3s ease",
        position: "relative", zIndex: 1,
      }}>

        {/* ── Header ── */}
        <header style={{
          padding: "12px 20px",
          borderBottom: `1px solid ${th.headerBorder}`,
          background: th.headerBg,
          backdropFilter: "blur(16px)",
          display: "flex", justifyContent: "space-between", alignItems: "center",
          position: "sticky", top: 0, zIndex: 20, flexShrink: 0,
        }}>
          {/* Brand */}
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{
              width: 34, height: 34, borderRadius: 10,
              background: isDark ? "rgba(99,102,241,0.2)" : "rgba(30,64,175,0.1)",
              border: `1px solid ${isDark ? "rgba(99,102,241,0.3)" : "rgba(30,64,175,0.2)"}`,
              display: "flex", alignItems: "center", justifyContent: "center",
            }}>
              <Shield size={16} color={th.accent} strokeWidth={2} />
            </div>
            <div>
              <div style={{ fontSize: "0.9rem", fontWeight: 700, letterSpacing: "0.04em", color: th.text }}>
                Salicru SPS 850 Home
              </div>
              <div style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.14em" }}>
                NUT MONITOR · NETWORK UPS TOOLS
              </div>
            </div>
          </div>

          {/* Right: alerts + status + toggle */}
          <div aria-live="polite" style={{ display: "flex", alignItems: "center", gap: 8 }}>
            {onBattery && (
              <div className="alert-new" style={alertPill("#dc2626")}>
                <AlertTriangle size={11} color="#dc2626" />
                <span>{lowBat ? "BAT. BAJA" : "EN BATERÍA"}</span>
              </div>
            )}
            {replaceBat && (
              <div className="alert-new" style={alertPill("#ea580c")}>
                <AlertTriangle size={11} color="#ea580c" />
                <span style={{ color: "#ea580c" }}>CAMBIAR BAT.</span>
              </div>
            )}
            {battHealth.level === "low" && (
              <div className="alert-new" style={alertPill("#d97706")}>
                <AlertTriangle size={11} color="#d97706" />
                <span style={{ color: "#d97706" }}>TENSIÓN BAJA</span>
              </div>
            )}

            {/* Dot + time */}
            <div style={{
              display: "flex", alignItems: "center", gap: 6,
              background: isDark ? "rgba(255,255,255,0.04)" : "rgba(0,0,0,0.04)",
              border: `1px solid ${th.border}`,
              borderRadius: 8, padding: "4px 10px",
            }}>
              <div className="dot-live" style={{
                width: 6, height: 6, borderRadius: "50%",
                background: isMock ? "#d97706" : "#16a34a",
                boxShadow: `0 0 6px ${isMock ? "#d97706" : "#16a34a"}`,
              }} />
              <span style={{ fontFamily: "'Fira Code', monospace", fontSize: "0.62rem", color: th.text2 }}>
                {ts.toLocaleTimeString()}
              </span>
              {isMock && (
                <span style={{
                  fontSize: "0.6rem", letterSpacing: "0.1em",
                  color: "#92400e", background: th.badgeMock,
                  border: "1px solid rgba(217,119,6,0.3)",
                  padding: "1px 5px", borderRadius: 4, fontWeight: 600,
                }}>MOCK</span>
              )}
            </div>

            <button className="theme-btn" onClick={toggleTheme} aria-label="Cambiar tema">
              {isDark
                ? <Sun size={16} color={th.text2} />
                : <Moon size={16} color={th.text2} />
              }
            </button>
          </div>
        </header>

        {/* ── Overload alert bar ── */}
        {overload && (
          <div role="alert" className="alert-new" style={{
            margin: "12px 20px 0",
            background: "#dc262610", border: "1px solid #dc262640",
            borderRadius: 12, padding: "10px 14px",
            display: "flex", alignItems: "center", gap: 8,
          }}>
            <AlertTriangle size={14} color="#dc2626" />
            <span style={{ fontSize: "0.72rem", color: "#dc2626", fontWeight: 600, letterSpacing: "0.06em" }}>
              SAI SOBRECARGADO — Reduce la carga conectada
            </span>
          </div>
        )}

        {/* ── Battery degraded alert bar ── */}
        {battHealth.level === "critical" && (
          <div role="alert" className="alert-new" style={{
            margin: "12px 20px 0",
            background: "#ea580c12", border: "1px solid #ea580c45",
            borderRadius: 12, padding: "10px 14px",
            display: "flex", alignItems: "center", gap: 8,
          }}>
            <AlertTriangle size={14} color="#ea580c" />
            <span style={{ fontSize: "0.72rem", color: "#ea580c", fontWeight: 600, letterSpacing: "0.06em" }}>
              BATERÍA DEGRADADA — Tensión {Number(data.battery_voltage).toFixed(2)} V bajo umbral recomendado. Considera reemplazo.
            </span>
          </div>
        )}

        {/* ── Main grid ── */}
        <main style={{
          flex: 1,
          display: "grid",
          gridTemplateColumns: wide ? "auto 1fr" : "1fr",
          gap: 16,
          padding: "16px 20px",
          alignItems: "start",
          overflow: "hidden",
          minWidth: 0,
        }}>

          {/* ── Left: ring card ── */}
          <Card th={th} style={{ padding: "28px 24px", display: "flex", flexDirection: "column", alignItems: "center", gap: 16 }}>
            <BatteryRing pct={battPct} displayPct={displayPct} mode={mode} th={th} />

            {/* Status row */}
            <div style={{
              display: "flex", gap: 0, width: "100%",
              borderTop: `1px solid ${th.border}`, paddingTop: 14,
              justifyContent: "space-around",
            }}>
              {[
                {
                  label: "ESTADO",
                  val: onBattery ? "ON BATTERY" : "ONLINE",
                  color: onBattery ? "#dc2626" : "#16a34a",
                },
                { label: "MODO", val: mode.label, color: mode.color },
              ].map(({ label, val, color }) => (
                <div key={label} style={{ textAlign: "center" }}>
                  <div style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.14em", marginBottom: 4, fontWeight: 500 }}>
                    {label}
                  </div>
                  <div style={{
                    fontFamily: "'Fira Code', monospace",
                    fontSize: "0.68rem", fontWeight: 700, color,
                    letterSpacing: "0.05em",
                    transition: "color 0.4s ease",
                  }}>{val}</div>
                </div>
              ))}
            </div>

            {/* Line AC status */}
            <div style={{
              width: "100%",
              background: onBattery
                ? isDark ? "rgba(220,38,38,0.1)" : "#fff5f5"
                : isDark ? "rgba(22,163,74,0.1)" : "#f0fdf4",
              border: `1px solid ${onBattery ? "rgba(220,38,38,0.25)" : "rgba(22,163,74,0.25)"}`,
              borderRadius: 10, padding: "9px 14px",
              display: "flex", alignItems: "center", justifyContent: "space-between",
              transition: "all 0.4s ease",
            }}>
              <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
                {onBattery
                  ? <WifiOff size={13} color="#dc2626" />
                  : <Wifi size={13} color="#16a34a" />
                }
                <span style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.14em", fontWeight: 500 }}>
                  LÍNEA AC
                </span>
              </div>
              <span style={{
                fontFamily: "'Fira Code', monospace",
                fontSize: "0.68rem", fontWeight: 700,
                color: onBattery ? "#dc2626" : "#16a34a",
              }}>
                {onBattery ? "CORTE DE LUZ" : "RED PRESENTE"}
              </span>
            </div>
          </Card>

          {/* ── Right ── */}
          <div style={{ display: "flex", flexDirection: "column", gap: 12, minWidth: 0, overflow: "hidden" }}>

            {/* 3×2 stat cards */}
            <div style={{ display: "grid", gridTemplateColumns: wide ? "repeat(3, 1fr)" : "repeat(2, 1fr)", gap: 10 }}>
              <StatCard icon={Zap}             label="V. Entrada"   value={data.input_voltage}   unit="V"   accent="#3b82f6"  th={th} />
              <StatCard icon={Radio}           label="Frecuencia"   value={data.input_frequency}  unit="Hz"  accent="#0ea5e9"  th={th} />
              <StatCard icon={BatteryCharging} label="V. Batería"   value={data.battery_voltage}  unit="V"   accent="#16a34a"  th={th} warn={battHealth.level !== "ok"} />
              <StatCard icon={Activity}        label="Carga SAI"    value={hasLoadData ? loadPct : null} unit="%"   accent="#7c3aed"  th={th} warn={hasLoadData && loadPct > 80} />
              <StatCard icon={Clock}           label="Autonomía"    value={data.battery_runtime}  unit="min" accent="#d97706"  th={th} warn={onBattery && (data.battery_runtime ?? 99) < 10} />
              <StatCard icon={Gauge}           label="Potencia nom" value={nominalW}              unit="W"   accent="#64748b"  th={th} />
            </div>

            {/* Power bar */}
            <PowerBar loadPct={loadPct} nominalW={nominalW} hasData={hasLoadData} th={th} />

            {/* Flags + sparkline row */}
            <div style={{ display: "grid", gridTemplateColumns: wide ? "auto 1fr" : "1fr", gap: 10 }}>

              {/* Flags */}
              <Card th={th} style={{ padding: "14px 16px", minWidth: 170 }}>
                <SLabel th={th}>Flags NUT</SLabel>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                  {flags.length > 0
                    ? flags.map(f => <FlagBadge key={f.key} label={f.label} color={f.color} th={th} />)
                    : <span style={{ fontFamily: "'Fira Code', monospace", fontSize: "0.62rem", color: th.text3 }}>—</span>
                  }
                </div>
              </Card>

              {/* Sparkline */}
              <Card th={th} style={{ padding: "14px 16px" }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
                    <div style={{
                      width: 24, height: 24, borderRadius: 7,
                      background: `${bColor}20`, border: `1px solid ${bColor}30`,
                      display: "flex", alignItems: "center", justifyContent: "center",
                    }}>
                      <TrendingUp size={11} color={bColor} strokeWidth={2} />
                    </div>
                    <span style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.14em", fontWeight: 500 }}>
                      HISTORIAL BATERÍA
                    </span>
                  </div>
                  <div style={{ display: "flex", gap: 12 }}>
                    {[
                      { l: "MAX", v: history.length ? histMax + "%" : "—", c: "#16a34a" },
                      { l: "MIN", v: history.length ? histMin + "%" : "—", c: "#dc2626" },
                      { l: "AHORA", v: battPct + "%", c: bColor },
                    ].map(({ l, v, c }) => (
                      <div key={l} style={{ textAlign: "right" }}>
                        <div style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.12em", fontWeight: 500 }}>{l}</div>
                        <div style={{ fontFamily: "'Fira Code', monospace", fontSize: "0.66rem", fontWeight: 700, color: c }}>{v}</div>
                      </div>
                    ))}
                  </div>
                </div>
                <div style={{ display: "flex" }}>
                  <Sparkline data={history} color={bColor} />
                </div>
              </Card>
            </div>
          </div>
        </main>

        {/* ── Footer ── */}
        <footer style={{
          padding: "8px 20px",
          borderTop: `1px solid ${th.headerBorder}`,
          display: "flex", justifyContent: "space-between", alignItems: "center",
          flexShrink: 0,
          background: th.headerBg, backdropFilter: "blur(16px)",
        }}>
          <span style={{ fontSize: "0.62rem", color: th.text3, letterSpacing: "0.1em" }}>
            NUT Monitor v2.1 · Salicru SPS 850 · {isMock ? "SIMULADO" : "EN VIVO"}
          </span>
          {data.battery_charge_low != null && (
            <span style={{ fontFamily: "'Fira Code', monospace", fontSize: "0.62rem", color: th.text3 }}>
              Umbral batería baja: {data.battery_charge_low}%
            </span>
          )}
        </footer>

      </div>
    </>
  );
}
