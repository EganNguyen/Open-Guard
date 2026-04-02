"use client";

import { useEffect, useState } from "react";
import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";
import { getPolicies, createPolicy, deletePolicy, Policy } from "@/lib/api";

const POLICY_TYPES = [
  { value: "ip_allowlist", label: "IP Allowlist" },
  { value: "rbac", label: "RBAC (Role-Based)" },
  { value: "data_export", label: "Data Export" },
  { value: "anon_access", label: "Anonymous Access" },
  { value: "session_limit", label: "Session Limit" },
];

function buildRules(type: string, rawRules: string): object {
  try {
    return JSON.parse(rawRules);
  } catch {
    // Fallback default rules per type
    if (type === "ip_allowlist") return { allowed_ips: [] };
    if (type === "rbac") return { allowed_roles: [] };
    return {};
  }
}

export default function AccessPoliciesPage() {
  const [policies, setPolicies] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showCreate, setShowCreate] = useState(false);

  // Form state
  const [policyName, setPolicyName] = useState("");
  const [policyType, setPolicyType] = useState("ip_allowlist");
  const [policyRules, setPolicyRules] = useState('{"allowed_ips":["1.2.3.4"]}');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  useEffect(() => {
    loadPolicies();
  }, []);

  const loadPolicies = async () => {
    try {
      setLoading(true);
      setError("");
      const token = localStorage.getItem("access_token");
      if (!token) throw new Error("No token");
      const resp = await getPolicies(token);
      setPolicies(resp.data ?? []);
    } catch (err: any) {
      setError(err?.error?.message || "Failed to load policies");
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setCreateError("");
    try {
      const token = localStorage.getItem("access_token");
      if (!token) throw new Error("No token");
      await createPolicy(token, {
        name: policyName,
        type: policyType,
        rules: buildRules(policyType, policyRules),
        enabled: true,
      });
      setPolicyName("");
      setPolicyType("ip_allowlist");
      setPolicyRules('{"allowed_ips":["1.2.3.4"]}');
      setShowCreate(false);
      await loadPolicies();
    } catch (err: any) {
      setCreateError(err?.error?.message || "Failed to create policy");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    const token = localStorage.getItem("access_token");
    if (!token) return;
    try {
      await deletePolicy(token, id);
      await loadPolicies();
    } catch (err: any) {
      setError(err?.error?.message || "Failed to delete policy");
    }
  };

  return (
    <DashboardLayout title="Access policies">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Policy Registry
            <span className="tag tag-amber" style={{ marginLeft: "8px" }}>Phase 2</span>
          </span>
          <button
            className="btn btn-primary"
            style={{ fontSize: "12px" }}
            onClick={() => setShowCreate(!showCreate)}
            data-testid="toggle-create-policy"
          >
            {showCreate ? "Cancel" : "+ Create policy"}
          </button>
        </div>

        {error && (
          <div className="alert alert-error" style={{ margin: "16px" }}>{error}</div>
        )}

        {showCreate && (
          <div style={{ padding: "16px", borderBottom: "1px solid var(--border)" }}>
            <h4 style={{ marginBottom: "16px" }}>Create New Policy</h4>
            <form onSubmit={handleCreate} style={{ display: "grid", gap: "12px" }}>
              {createError && <div className="alert alert-error">{createError}</div>}
              <div>
                <label style={{ display: "block", fontSize: "12px", color: "var(--muted)", marginBottom: "4px" }}>
                  Policy Name
                </label>
                <input
                  id="policy-name"
                  className={styles.input}
                  placeholder="e.g. IP Allowlist - Production"
                  value={policyName}
                  onChange={e => setPolicyName(e.target.value)}
                  style={{ width: "100%" }}
                  required
                />
              </div>
              <div>
                <label style={{ display: "block", fontSize: "12px", color: "var(--muted)", marginBottom: "4px" }}>
                  Policy Type
                </label>
                <select
                  id="policy-type"
                  className={styles.input}
                  value={policyType}
                  onChange={e => {
                    setPolicyType(e.target.value);
                    if (e.target.value === "ip_allowlist") setPolicyRules('{"allowed_ips":["1.2.3.4"]}');
                    else if (e.target.value === "rbac") setPolicyRules('{"allowed_roles":["admin"]}');
                    else if (e.target.value === "data_export") setPolicyRules('{"allowed_roles":["admin"]}');
                    else if (e.target.value === "anon_access") setPolicyRules('{"allow_anonymous":false}');
                    else setPolicyRules('{}');
                  }}
                  style={{ width: "100%" }}
                >
                  {POLICY_TYPES.map(t => (
                    <option key={t.value} value={t.value}>{t.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label style={{ display: "block", fontSize: "12px", color: "var(--muted)", marginBottom: "4px" }}>
                  Rules (JSON)
                </label>
                <textarea
                  id="policy-rules"
                  className={styles.input}
                  value={policyRules}
                  onChange={e => setPolicyRules(e.target.value)}
                  style={{ width: "100%", minHeight: "80px", fontFamily: "var(--mono)", fontSize: "12px" }}
                />
              </div>
              <button
                type="submit"
                className="btn btn-primary"
                id="submit-policy"
                style={{ width: "fit-content" }}
                disabled={creating}
                data-testid="submit-policy"
              >
                {creating ? "Saving..." : "Save Policy"}
              </button>
            </form>
          </div>
        )}

        {loading ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>Loading...</div>
        ) : policies.length === 0 ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>📑</div>
            <h3>No policies configured</h3>
            <p style={{ maxWidth: "400px", margin: "0 auto" }}>
              Create your first policy to control access based on IP, roles, or context.
            </p>
          </div>
        ) : (
          <table className={styles.auditTable} data-testid="policy-list">
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Status</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {policies.map((p: any) => (
                <tr key={p.id}>
                  <td>{p.name}</td>
                  <td>
                    <span className="tag tag-blue" style={{ fontSize: "10px" }}>{p.type}</span>
                  </td>
                  <td>
                    <span className={`tag tag-${p.enabled ? "green" : "amber"}`}>
                      {p.enabled ? "active" : "disabled"}
                    </span>
                  </td>
                  <td className={styles.timeCell}>{new Date(p.created_at).toLocaleString()}</td>
                  <td>
                    <button
                      className="btn btn-secondary"
                      style={{ fontSize: "11px", padding: "4px 10px" }}
                      onClick={() => handleDelete(p.id)}
                      data-testid={`delete-policy-${p.id}`}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </DashboardLayout>
  );
}
