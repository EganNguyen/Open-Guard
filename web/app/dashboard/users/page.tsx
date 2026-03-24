"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function UsersPage() {
  return (
    <DashboardLayout title="Users">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Organization Users
            <span className="tag tag-green" style={{ marginLeft: "8px" }}>Phase 1</span>
          </span>
          <button className="btn btn-primary" style={{ fontSize: "12px" }}>+ Add user</button>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>👥</div>
          <h3>User Management</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Manage your organization's users, groups, and administrative roles.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
