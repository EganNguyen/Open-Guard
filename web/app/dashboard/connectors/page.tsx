"use client";

import { useEffect, useState } from "react";
import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";
import { getConnectors, createConnector, Connector } from "@/lib/api";

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [newAppName, setNewAppName] = useState("");
  const [newWebhookUrl, setNewWebhookUrl] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    loadConnectors();
  }, []);

  const loadConnectors = async () => {
    try {
      setLoading(true);
      const token = localStorage.getItem("access_token");
      if (!token) throw new Error("No token");
      const resp = await getConnectors(token);
      setConnectors(resp.data);
    } catch (err: any) {
      console.error("Failed to load connectors:", err);
      setError("Failed to load connectors");
    } finally {
      setLoading(false);
    }
  };

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const token = localStorage.getItem("access_token");
      if (!token) throw new Error("No token");
      await createConnector(token, { name: newAppName, webhook_url: newWebhookUrl });
      setShowModal(false);
      setNewAppName("");
      setNewWebhookUrl("");
      loadConnectors();
    } catch (err: any) {
      setError(err.error?.message || "Failed to register app");
    }
  };

  return (
    <DashboardLayout title="Connectors">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Connected Applications
          </span>
          <button 
            className="btn btn-primary" 
            style={{ fontSize: "12px" }} 
            onClick={() => setShowModal(true)}
            data-testid="register-app-btn"
          >
            + Register app
          </button>
        </div>

        {loading ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>Loading...</div>
        ) : connectors.length === 0 ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>🔌</div>
            <h3>No connectors found</h3>
            <p style={{ maxWidth: "400px", margin: "0 auto" }}>
              Register your external services to receive signed webhooks and 
              interact with the OpenGuard Control Plane.
            </p>
          </div>
        ) : (
          <table className={styles.auditTable}>
            <thead>
              <tr>
                <th>Name</th>
                <th>Webhook URL</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody data-testid="connector-list">
              {connectors.map(c => (
                <tr key={c.id}>
                  <td>{c.name}</td>
                  <td style={{ color: "var(--muted)", fontSize: "11px" }}>{c.webhook_url}</td>
                  <td>
                    <span className={`tag tag-${c.status === 'active' ? 'green' : 'amber'}`}>
                      {c.status}
                    </span>
                  </td>
                  <td className={styles.timeCell}>{new Date(c.created_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showModal && (
        <div className="modal-backdrop" style={{
          position: "fixed", top: 0, left: 0, right: 0, bottom: 0,
          background: "rgba(0,0,0,0.5)", display: "flex", alignItems: "center", justifyContent: "center",
          zIndex: 1000
        }}>
          <div className={styles.panel} style={{ width: "400px", padding: "24px" }}>
            <h3 style={{ marginBottom: "16px" }}>Register New Connector</h3>
            <form onSubmit={handleRegister}>
              {error && <div className="alert alert-error" style={{ marginBottom: "16px" }}>{error}</div>}
              <div className="form-group" style={{ marginBottom: "16px" }}>
                <label style={{ display: "block", marginBottom: "6px", fontSize: "12px" }}>App Name</label>
                <input 
                  type="text" 
                  className="input" 
                  placeholder="e.g. Main Storefront" 
                  value={newAppName} 
                  onChange={e => setNewAppName(e.target.value)}
                  data-testid="app-name-input"
                  required
                />
              </div>
              <div className="form-group" style={{ marginBottom: "20px" }}>
                <label style={{ display: "block", marginBottom: "6px", fontSize: "12px" }}>Webhook URL</label>
                <input 
                  type="url" 
                  className="input" 
                  placeholder="https://..." 
                  value={newWebhookUrl} 
                  onChange={e => setNewWebhookUrl(e.target.value)}
                  data-testid="app-webhook-input"
                  required
                />
              </div>
              <div style={{ display: "flex", gap: "12px", justifyContent: "flex-end" }}>
                <button type="button" className="btn btn-secondary" onClick={() => setShowModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" data-testid="submit-app-btn">Register</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </DashboardLayout>
  );
}
