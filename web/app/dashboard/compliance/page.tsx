"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function CompliancePage() {
  return (
    <DashboardLayout title="Compliance">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Compliance Reports
            <span className="tag tag-purple" style={{ marginLeft: "8px" }}>Phase 5</span>
          </span>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>🏆</div>
          <h3>System Compliance coming in Phase 5</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Generate GDPR, HIPAA, and SOC2 reports based on real-time ClickHouse analytics.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
