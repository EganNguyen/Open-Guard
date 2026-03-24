"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function AuditPage() {
  return (
    <DashboardLayout title="Audit log">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            System Audit Events
            <span className="tag tag-green" style={{ marginLeft: "8px" }}>Phase 3</span>
          </span>
        </div>
        <div style={{ padding: "20px" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "14px" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--border)", color: "var(--muted)", textAlign: "left" }}>
                <th style={{ padding: "12px" }}>Timestamp</th>
                <th style={{ padding: "12px" }}>Actor</th>
                <th style={{ padding: "12px" }}>Action</th>
                <th style={{ padding: "12px" }}>Target</th>
                <th style={{ padding: "12px" }}>Status</th>
              </tr>
            </thead>
            <tbody data-testid="audit-list">
              <tr style={{ borderBottom: "1px solid var(--border-light)" }}>
                <td style={{ padding: "12px" }}>2026-03-24 10:45:12</td>
                <td style={{ padding: "12px" }}>admin@openguard.io</td>
                <td style={{ padding: "12px" }}>policy.create</td>
                <td style={{ padding: "12px" }}>pol_rbac_01</td>
                <td style={{ padding: "12px" }}><span className="tag tag-green">Success</span></td>
              </tr>
              <tr style={{ borderBottom: "1px solid var(--border-light)" }}>
                <td style={{ padding: "12px" }}>2026-03-24 10:48:05</td>
                <td style={{ padding: "12px" }}>system</td>
                <td style={{ padding: "12px" }}>threat.detected</td>
                <td style={{ padding: "12px" }}>brute_force_attack</td>
                <td style={{ padding: "12px" }}><span className="tag tag-red">Alert</span></td>
              </tr>
            </tbody>
          </table>

          <div style={{ padding: "20px", textAlign: "center", color: "var(--muted)", marginTop: "20px" }}>
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>📜</div>
            <h3>High-performance log viewer coming in Phase 3/6</h3>
            <p style={{ maxWidth: "400px", margin: "0 auto" }}>
              The full audit log will support virtual scrolling for 10,000+ events and 
              cryptographic integrity verification.
            </p>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
