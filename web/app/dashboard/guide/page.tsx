"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "./guide.module.css";

/* ── Icons ── */
const ShieldIcon = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
  </svg>
);

const CheckCircleIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
    <polyline points="22 4 12 14.01 9 11.01"/>
  </svg>
);

const AlertTriangleIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
    <line x1="12" y1="9" x2="12" y2="13"/>
    <line x1="12" y1="17" x2="12.01" y2="17"/>
  </svg>
);

const CodeIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="16 18 22 12 16 6"/>
    <polyline points="8 6 2 12 8 18"/>
  </svg>
);

export default function SecurityGuidePage() {
  return (
    <DashboardLayout title="Security Guide">
      <div className={styles.guideContainer}>
        {/* Header */}
        <header className={styles.header}>
          <h1>Security Engineering Guide</h1>
          <p>
            The authoritative standards for OpenGuard enterprise implementation. 
            Every line of code must satisfy these quality and security requirements.
          </p>
        </header>

        {/* Core Philosophy */}
        <section className={styles.section}>
          <h2><ShieldIcon /> Core Philosophy</h2>
          <div className={styles.philosophyGrid}>
            <div className={styles.phiCard}>
              <span className={styles.phiIcon}>👁️</span>
              <h4>Readability First</h4>
              <p>Code is read 10x more than written. Optimize for the SRE debugging at 3 AM.</p>
            </div>
            <div className={styles.phiCard}>
              <span className={styles.phiIcon}>😴</span>
              <h4>Boring is Better</h4>
              <p>Resist the urge to be clever. Go is deliberately unexciting; follow that intent.</p>
            </div>
            <div className={styles.phiCard}>
              <span className={styles.phiIcon}>⚖️</span>
              <h4>System-Wide Consistency</h4>
              <p>Consistency beats local optimality. Stick to agreed patterns over clever shortcuts.</p>
            </div>
          </div>
        </section>

        {/* Non-Negotiable Rules */}
        <section className={styles.section}>
          <h2><AlertTriangleIcon /> Non-Negotiable Rules</h2>
          <div className={styles.ruleList}>
            <div className={styles.ruleItem}>
              <div className={styles.ruleIcon}><CheckCircleIcon /></div>
              <div className={styles.ruleContent}>
                <h4>Transactional Outbox Pattern</h4>
                <p>Every Kafka publish MUST go through the Outbox relay — never a direct producer call from a business handler.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleIcon}><CheckCircleIcon /></div>
              <div className={styles.ruleContent}>
                <h4>Row-Level Security (RLS)</h4>
                <p>Every table holding organization data must have RLS enabled and strictly enforced at the database level.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleIcon}><CheckCircleIcon /></div>
              <div className={styles.ruleContent}>
                <h4>Fail-Closed Enforcement</h4>
                <p>Default security state is DENY. If the policy engine is unavailable, all access must be blocked.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleIcon}><CheckCircleIcon /></div>
              <div className={styles.ruleContent}>
                <h4>Circuit Breakers</h4>
                <p>Every inter-service HTTP call must be wrapped in a circuit breaker to ensure system resilience.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleIcon}><CheckCircleIcon /></div>
              <div className={styles.ruleContent}>
                <h4>No Unparameterized SQL</h4>
                <p>String concatenation in SQL is strictly forbidden. Use parameterized queries only, enforced by CI linters.</p>
              </div>
            </div>
          </div>
        </section>

        {/* Architecture Standards */}
        <section className={styles.section}>
          <h2><CodeIcon /> Implementation Standards</h2>
          <div className={styles.cardGrid}>
            <div className={styles.card}>
              <h3>Package Design</h3>
              <p>One coherent concept per package. If you can&apos;t describe it in one sentence without &quot;and&quot;, it needs a split.</p>
            </div>
            <div className={styles.card}>
              <h3>Error Discipline</h3>
              <p>Log or Return — never both. Wrap errors once at each layer boundary with relevant context.</p>
            </div>
            <div className={styles.card}>
              <h3>Dependency Injection</h3>
              <p>Constructor injection only. main.go is the wiring file. No globals, no singletons in business logic.</p>
            </div>
            <div className={styles.card}>
              <h3>Context Usage</h3>
              <p>Context as first parameter, always. Never pass context.Background() inside a request handler.</p>
            </div>
            <div className={styles.card}>
              <h3>Observability</h3>
              <p>Structured logging with slog. Start a tracing span at every service call boundary.</p>
            </div>
            <div className={styles.card}>
              <h3>Testing</h3>
              <p>Test behavior, not implementation. Integration tests use real databases via testcontainers-go.</p>
            </div>
          </div>
        </section>

        {/* Frontend Standards */}
        <section className={styles.section}>
          <h2><div className={styles.ruleIcon} style={{ width: 18, height: 18, marginRight: 0 }}><ShieldIcon /></div> Frontend (Next.js) Standards</h2>
          <div className={styles.ruleList}>
            <div className={styles.ruleItem}>
              <div className={styles.ruleContent}>
                <h4>Real-Time Performance</h4>
                <p>Use Server-Sent Events (SSE) for metrics and threat updates. Polling is not acceptable for real-time data.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleContent}>
                <h4>Virtual Scrolling</h4>
                <p>All large data tables (Audit Logs) must use virtual scrolling. Fetch cursor-paginated chunks of 100 rows.</p>
              </div>
            </div>
            <div className={styles.ruleItem}>
              <div className={styles.ruleContent}>
                <h4>Strict Security Headers</h4>
                <p>Strict Content Security Policy (CSP), X-Frame-Options (DENY), and HSTS must be present on every response.</p>
              </div>
            </div>
          </div>
        </section>
      </div>
    </DashboardLayout>
  );
}
