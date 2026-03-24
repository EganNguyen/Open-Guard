"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function AccessPoliciesPage() {
  return (
    <DashboardLayout title="Access policies">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Policy Registry
            <span className="tag tag-amber" style={{ marginLeft: "8px" }}>Phase 2</span>
          </span>
          <button className="btn btn-primary" style={{ fontSize: "12px" }}>+ Create policy</button>
        </div>
        <div style={{ padding: "20px" }}>
          <div style={{ border: "1px solid var(--border)", borderRadius: "8px", padding: "20px", marginBottom: "20px" }}>
            <h4 style={{ marginBottom: "16px" }}>Create New Policy</h4>
            <div style={{ display: "grid", gap: "12px" }}>
              <div>
                <label style={{ display: "block", fontSize: "12px", color: "var(--muted)", marginBottom: "4px" }}>Policy Name</label>
                <input id="policy-name" className={styles.input} placeholder="e.g. S3 Bucket Filter" style={{ width: "100%" }} />
              </div>
              <div>
                <label style={{ display: "block", fontSize: "12px", color: "var(--muted)", marginBottom: "4px" }}>Effect</label>
                <select id="policy-effect" className={styles.input} style={{ width: "100%" }}>
                  <option value="Allow">Allow</option>
                  <option value="Deny">Deny</option>
                </select>
              </div>
              <button className="btn btn-primary" id="submit-policy" style={{ width: "fit-content" }}>
                Save Policy
              </button>
            </div>
          </div>

          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>📑</div>
            <h3>RBAC & ABAC Controls</h3>
            <p style={{ maxWidth: "400px", margin: "0 auto" }}>
              Define granular access control rules based on user roles, attributes, and context.
            </p>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
