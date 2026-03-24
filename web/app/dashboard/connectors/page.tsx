"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function ConnectorsPage() {
  return (
    <DashboardLayout title="Connectors">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Connected Applications
            <span className="tag tag-purple" style={{ marginLeft: "8px" }}>Phase 6</span>
          </span>
          <button className="btn btn-primary" style={{ fontSize: "12px" }}>+ Register app</button>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>🔌</div>
          <h3>Connector Registry coming in Phase 6</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Register your external services to receive signed webhooks and 
            interact with the OpenGuard Control Plane.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
