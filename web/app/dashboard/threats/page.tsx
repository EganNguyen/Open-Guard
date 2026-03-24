"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function ThreatsPage() {
  return (
    <DashboardLayout title="Threat detection">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Live Threat Stream 
            <span className="tag tag-red" style={{ marginLeft: "8px" }}>Phase 4</span>
          </span>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>📡</div>
          <h3>Real-time monitoring coming in Phase 4</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            This page will use Server-Sent Events (SSE) to deliver instant alerts for brute force attacks, 
            impossible travel, and privilege escalation.
          </p>
          <button className="btn btn-primary" style={{ marginTop: "20px" }} disabled>
            Enable SSE Stream
          </button>
        </div>
      </div>
    </DashboardLayout>
  );
}
