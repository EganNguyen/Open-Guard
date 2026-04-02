"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function AuthPoliciesPage() {
  return (
    <DashboardLayout title="Authentication policies">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Auth Configuration
            <span className="tag tag-purple" style={{ marginLeft: "8px" }}>Phase 6</span>
          </span>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>🛡️</div>
          <h3>Authentication Governance</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Configure MFA requirements, session lifetimes, and password complexity rules.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
