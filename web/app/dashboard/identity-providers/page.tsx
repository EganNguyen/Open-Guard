"use client";

import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";

export default function IdentityProvidersPage() {
  return (
    <DashboardLayout title="Identity providers">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            External IdPs
            <span className="tag tag-green" style={{ marginLeft: "8px" }}>Phase 1</span>
          </span>
          <button className="btn btn-primary" style={{ fontSize: "12px" }}>+ Add provider</button>
        </div>
        <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
          <div style={{ fontSize: "48px", marginBottom: "16px" }}>🔑</div>
          <h3>Identity Federation</h3>
          <p style={{ maxWidth: "400px", margin: "0 auto" }}>
            Connect OIDC (Okta, Google, Azure AD) or SAML 2.0 Identity Providers.
          </p>
        </div>
      </div>
    </DashboardLayout>
  );
}
