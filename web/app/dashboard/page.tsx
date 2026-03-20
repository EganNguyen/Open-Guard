"use client";

import { useState } from "react";
import DashboardLayout from "@/components/DashboardLayout";
import styles from "./dashboard.module.css";

/* ── Sparkline bars ── */
function Sparkline({ data, color }: { data: number[]; color: string }) {
  return (
    <div className={styles.sparklineRow}>
      {data.map((h, i) => (
        <div key={i} className={styles.sparkBar} style={{ height: `${h}%`, background: color }} />
      ))}
    </div>
  );
}

/* ── Check icon ── */
const CheckIcon = () => (
  <svg width="10" height="10" viewBox="0 0 20 20" fill="currentColor">
    <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"/>
  </svg>
);

/* ── Recommendation item ── */
function RecItem({ title, desc, meta, done }: { title: string; desc: string; meta?: string; done?: boolean }) {
  const [isDone, setIsDone] = useState(!!done);
  return (
    <div className={`${styles.recItem} ${isDone ? styles.recDone : ""}`} onClick={() => setIsDone(!isDone)}>
      <div className={styles.recCheck}><CheckIcon /></div>
      <div className={styles.recContent}>
        <div className={styles.recTitle} style={isDone ? { color: "var(--muted)", textDecoration: "line-through" } : {}}>{title}</div>
        <div className={styles.recDesc}>{desc}</div>
      </div>
      {meta && <div className={styles.recMeta} style={isDone ? { color: "var(--green)" } : {}}>{isDone ? "✓ Done" : meta}</div>}
      <div className={styles.recArrow}>›</div>
    </div>
  );
}

export default function DashboardPage() {
  const [hideCompleted, setHideCompleted] = useState(false);

  return (
    <DashboardLayout title="Security guide">
      {/* ── HERO GRID ── */}
      <div className={styles.heroGrid}>
        {/* First step */}
        <div className={styles.heroCard}>
          <div className={styles.eyebrow}>First step</div>
          <h2>Verify your domain</h2>
          <p>Prove you own your domain so you can claim and manage user accounts. Managed accounts are significantly more secure and enable policy enforcement.</p>
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <button className="btn btn-primary">
              <svg width="13" height="13" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"/>
              </svg>
              Verify domain
            </button>
            <button className="btn btn-ghost">Learn more</button>
          </div>
          <div className={styles.progressWrap}>
            <div className={styles.progressLabel}>
              <span>Setup progress</span>
              <span>2 of 8 complete</span>
            </div>
            <div className={styles.progressBar}>
              <div className={styles.progressFill} style={{ width: "25%" }} />
            </div>
          </div>
        </div>

        {/* Users chart */}
        <div className={styles.heroCard} style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "14px" }}>
            <span style={{ fontSize: "13.5px", fontWeight: 600 }}>Users with access to your apps</span>
            <div style={{ display: "flex", gap: "6px" }}>
              <div className={styles.iconBtn}>↻</div>
              <div className={styles.iconBtn}>⋯</div>
            </div>
          </div>
          <div className="chip-group" style={{ marginBottom: "14px" }}>
            <div className="chip active"><div className="chip-dot" style={{ background: "var(--accent)" }} />All users</div>
            <div className="chip"><div className="chip-dot" style={{ background: "var(--green)" }} />Managed</div>
            <div className="chip"><div className="chip-dot" style={{ background: "var(--amber)" }} />External</div>
          </div>
          <div className={styles.usersChartArea}>
            <div className={styles.usersEmptyIcon}>
              <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z"/>
              </svg>
            </div>
            <span style={{ fontSize: "13px", color: "var(--muted)" }}>No insights yet — verify your domain first</span>
            <button className="btn btn-ghost" style={{ fontSize: "12px", padding: "6px 12px", marginTop: "4px" }}>Verify domain →</button>
          </div>
        </div>
      </div>

      {/* ── STATS ROW ── */}
      <div className={styles.statsRow}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}><div className={styles.statDot} style={{ background: "var(--red)" }} />Active alerts</div>
          <div className={styles.statValue} style={{ color: "var(--red)" }}>3</div>
          <div className={styles.statSub}><span className={`${styles.statTrend} ${styles.trendUp}`}>+2</span> since yesterday</div>
          <Sparkline data={[30, 20, 60, 40, 80, 50, 100]} color="var(--red)" />
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}><div className={styles.statDot} style={{ background: "var(--accent)" }} />Managed users</div>
          <div className={styles.statValue} style={{ color: "var(--accent)" }}>0</div>
          <div className={styles.statSub}><span className={`${styles.statTrend} ${styles.trendNeutral}`}>—</span> pending domain verification</div>
          <Sparkline data={[10, 10, 10, 10, 10, 10, 10]} color="var(--accent)" />
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}><div className={styles.statDot} style={{ background: "var(--green)" }} />Audit events (24h)</div>
          <div className={styles.statValue} style={{ color: "var(--green)" }}>1,284</div>
          <div className={styles.statSub}><span className={`${styles.statTrend} ${styles.trendDown}`}>-12%</span> vs prior day</div>
          <Sparkline data={[55, 70, 45, 90, 60, 75, 80]} color="var(--green)" />
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}><div className={styles.statDot} style={{ background: "var(--purple)" }} />Policy evaluations</div>
          <div className={styles.statValue} style={{ color: "var(--purple)" }}>48.2k</div>
          <div className={styles.statSub}><span className={`${styles.statTrend} ${styles.trendDown}`}>p99 22ms</span> all passing</div>
          <Sparkline data={[65, 80, 70, 95, 75, 85, 100]} color="var(--purple)" />
        </div>
      </div>

      {/* ── SECTION GRID ── */}
      <div className={styles.sectionGrid}>
        {/* Recommendations */}
        <div className={styles.panel}>
          <div className={styles.panelHeader}>
            <span className={styles.panelTitle}>Security recommendations</span>
            <div className={styles.panelActions}>
              <div style={{ display: "flex", alignItems: "center", gap: "6px", fontSize: "12px", color: "var(--muted)" }}>
                Hide completed
                <div
                  className={styles.toggle}
                  onClick={() => setHideCompleted(!hideCompleted)}
                  style={{ background: hideCompleted ? "var(--accent)" : "var(--border2)" }}
                >
                  <div className={styles.toggleThumb} style={{ left: hideCompleted ? "14px" : "2px" }} />
                </div>
              </div>
            </div>
          </div>

          <div className={styles.recSectionLabel}>Get control of your organization</div>
          <RecItem title="Add another admin" desc="Ensure you have another admin to avoid being locked out" meta="1 org admin" />
          <RecItem title="Verify your domain" desc="Prove you own the domain of your user accounts" />
          <RecItem title="Claim your user accounts" desc="Claim accounts from your domain to apply authentication settings" />
          <RecItem title="Update your authentication policy" desc="Specify authentication settings for managed accounts" />
          <RecItem title="Control the location of your data" desc="Choose where you store app data to meet privacy and legal requirements" />

          <div className={styles.recSectionLabel} style={{ paddingTop: "6px" }}>
            Secure your organization&apos;s users and data
            <span style={{ color: "var(--accent)", fontSize: "10px", marginLeft: "6px", fontWeight: 500, cursor: "pointer" }}>⚡ Powered by OpenGuard</span>
          </div>
          <RecItem title="Connect your identity provider" desc="Set up SAML SSO and automatic user provisioning via SCIM" meta="0 providers" />
          {!hideCompleted && <RecItem title="Set up MFA enforcement" desc="Require two-factor authentication for all managed accounts" done />}
          <RecItem title="Set up external user policy" desc="Control how you manage users you don't own" meta="0 external users" />
        </div>

        {/* Right column */}
        <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
          {/* Active Alerts */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <span className={styles.panelTitle}>Active alerts</span>
              <div className={styles.panelActions}>
                <span className="tag tag-red">3 open</span>
                <div className={styles.iconBtn} style={{ fontSize: "11px" }}>→</div>
              </div>
            </div>
            <AlertItem severity="critical" title="Brute force detected" tag="CRITICAL" tagClass="tag-red" actor="user@acme.com" time="2m ago" />
            <AlertItem severity="high" title="Impossible travel" tag="HIGH" tagClass="tag-amber" actor="bob@acme.com" time="18m ago" />
            <AlertItem severity="medium" title="Off-hours admin access" tag="MEDIUM" tagClass="tag-purple" actor="svc-deploy" time="1h ago" />
          </div>

          {/* SLO Status */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <span className={styles.panelTitle}>Service SLOs</span>
              <span className="tag tag-green">All healthy</span>
            </div>
            <div className={styles.sloGrid}>
              <SloItem name="Login p99" val="88ms" target="target <150ms" />
              <SloItem name="Policy p99" val="22ms" target="target <30ms" />
              <SloItem name="Audit p99" val="64ms" target="target <100ms" />
              <SloItem name="JWT valid p99" val="3ms" target="target <5ms" />
              <SloItem name="Outbox lag" val="824" target="target <1000" warn />
              <SloItem name="CB status" val="Closed" target="all breakers" />
            </div>
          </div>

          {/* Audit Mini Table */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <span className={styles.panelTitle}>Recent audit events</span>
              <div className={styles.iconBtn} style={{ fontSize: "11px" }}>→</div>
            </div>
            <table className={styles.auditTable}>
              <thead>
                <tr>
                  <th>Actor</th>
                  <th>Event</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                <AuditRow initials="JD" name="Jane D." gradient="linear-gradient(135deg,#4f7cff,#a78bfa)" event="auth.login.success" time="2m ago" />
                <AuditRow initials="BK" name="Bob K." gradient="linear-gradient(135deg,#22d98f,#4f7cff)" event="policy.changes" time="15m ago" />
                <AuditRow initials="SY" name="system" gradient="linear-gradient(135deg,#f5a623,#ff4f6b)" event="auth.mfa.enrolled" time="42m ago" />
                <AuditRow initials="AP" name="Alice P." gradient="linear-gradient(135deg,#a78bfa,#22d98f)" event="user.created" time="1h ago" />
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}

/* ── Sub-components ── */

function AlertItem({ severity, title, tag, tagClass, actor, time }: {
  severity: string; title: string; tag: string; tagClass: string; actor: string; time: string;
}) {
  const sevClass =
    severity === "critical" ? styles.sevCritical :
    severity === "high" ? styles.sevHigh :
    severity === "medium" ? styles.sevMedium : styles.sevLow;

  return (
    <div className={styles.alertItem}>
      <div className={`${styles.alertSev} ${sevClass}`} />
      <div className={styles.alertBody}>
        <div className={styles.alertTitle}>{title}</div>
        <div className={styles.alertMeta}>
          <span className={`tag ${tagClass}`}>{tag}</span>
          <span>{actor}</span>
          <span style={{ marginLeft: "auto", fontFamily: "var(--mono)", fontSize: "10.5px" }}>{time}</span>
        </div>
      </div>
    </div>
  );
}

function SloItem({ name, val, target, warn, crit }: {
  name: string; val: string; target: string; warn?: boolean; crit?: boolean;
}) {
  const cls = crit ? styles.sloCrit : warn ? styles.sloWarn : "";
  return (
    <div className={styles.sloItem}>
      <div className={styles.sloName}>{name}</div>
      <div className={`${styles.sloVal} ${cls}`}>{val}</div>
      <div className={styles.sloTarget}>{target}</div>
    </div>
  );
}

function AuditRow({ initials, name, gradient, event, time }: {
  initials: string; name: string; gradient: string; event: string; time: string;
}) {
  return (
    <tr>
      <td>
        <div className={styles.actorCell}>
          <div className={styles.miniAvatar} style={{ background: gradient }}>{initials}</div>
          {name}
        </div>
      </td>
      <td><span className={styles.eventType}>{event}</span></td>
      <td className={styles.timeCell}>{time}</td>
    </tr>
  );
}
