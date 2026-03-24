"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function ExternalUsersPage() {
  return (
    <DashboardLayout title="External users">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            External Collaboration
            <span className="tag tag-purple" style={{ marginLeft: "8px" }}>Phase 6</span>
          </span>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>🌐</div>
          <h3>External User Access</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Manage access for contractors, partners, and guest users from external domains.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
