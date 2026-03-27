"use client";

import { useState, useEffect, useCallback } from "react";
import DashboardLayout from "@/components/DashboardLayout";
import {
  getUsers,
  createUser,
  suspendUser,
  activateUser,
  deleteUser,
  User,
  CreateUserRequest,
} from "@/lib/api";
import styles from "../dashboard.module.css";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("access_token");
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function StatusBadge({ status }: { status: string }) {
  const color =
    status === "active"
      ? "tag-green"
      : status === "suspended"
      ? "tag-red"
      : "tag-yellow";
  return (
    <span className={`tag ${color}`} style={{ textTransform: "capitalize" }}>
      {status}
    </span>
  );
}

interface CreateUserModalProps {
  onClose: () => void;
  onCreated: (user: User) => void;
}

function CreateUserModal({ onClose, onCreated }: CreateUserModalProps) {
  const [form, setForm] = useState<CreateUserRequest>({
    email: "",
    display_name: "",
    password: "TempPassword123!",
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const token = getToken();
      if (!token) throw new Error("Not authenticated");
      const user = await createUser(token, form);
      onCreated(user);
    } catch (err: any) {
      setError(err?.error?.message ?? err?.message ?? "Failed to create user");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.5)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 1000,
      }}
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        style={{
          background: "var(--card)",
          border: "1px solid var(--border)",
          borderRadius: "12px",
          padding: "28px",
          width: "100%",
          maxWidth: "460px",
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "20px" }}>
          <h3 style={{ margin: 0 }}>Add User to Organization</h3>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              color: "var(--muted)",
              cursor: "pointer",
              fontSize: "20px",
            }}
          >
            ×
          </button>
        </div>

        {error && (
          <div
            style={{
              background: "rgba(239,68,68,0.1)",
              border: "1px solid rgba(239,68,68,0.3)",
              borderRadius: "8px",
              padding: "10px 14px",
              color: "#ef4444",
              marginBottom: "16px",
              fontSize: "13px",
            }}
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
          <div>
            <label style={{ display: "block", fontSize: "13px", color: "var(--muted)", marginBottom: "6px" }}>
              Email Address *
            </label>
            <input
              data-testid="create-user-email"
              type="email"
              required
              value={form.email}
              onChange={(e) => setForm({ ...form, email: e.target.value })}
              placeholder="user@example.com"
              style={{
                width: "100%",
                padding: "10px 12px",
                background: "var(--bg)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                color: "var(--fg)",
                fontSize: "14px",
                boxSizing: "border-box",
              }}
            />
          </div>

          <div>
            <label style={{ display: "block", fontSize: "13px", color: "var(--muted)", marginBottom: "6px" }}>
              Display Name
            </label>
            <input
              data-testid="create-user-display-name"
              type="text"
              value={form.display_name}
              onChange={(e) => setForm({ ...form, display_name: e.target.value })}
              placeholder="Jane Smith"
              style={{
                width: "100%",
                padding: "10px 12px",
                background: "var(--bg)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                color: "var(--fg)",
                fontSize: "14px",
                boxSizing: "border-box",
              }}
            />
          </div>

          <div>
            <label style={{ display: "block", fontSize: "13px", color: "var(--muted)", marginBottom: "6px" }}>
              Temporary Password
            </label>
            <input
              data-testid="create-user-password"
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              placeholder="Min 8 characters"
              style={{
                width: "100%",
                padding: "10px 12px",
                background: "var(--bg)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                color: "var(--fg)",
                fontSize: "14px",
                boxSizing: "border-box",
              }}
            />
          </div>

          <div style={{ display: "flex", gap: "10px", justifyContent: "flex-end", marginTop: "6px" }}>
            <button type="button" onClick={onClose} className="btn" style={{ fontSize: "13px" }}>
              Cancel
            </button>
            <button
              data-testid="submit-create-user"
              type="submit"
              className="btn btn-primary"
              disabled={loading}
              style={{ fontSize: "13px" }}
            >
              {loading ? "Creating…" : "Create User"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const loadUsers = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const token = getToken();
      if (!token) throw new Error("Not authenticated");
      const res = await getUsers(token);
      setUsers(res.data ?? []);
    } catch (err: any) {
      setError(err?.error?.message ?? err?.message ?? "Failed to load users");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadUsers();
  }, [loadUsers]);

  const handleCreated = (user: User) => {
    setUsers((prev) => [user, ...prev]);
    setShowModal(false);
  };

  const handleSuspend = async (user: User) => {
    setActionLoading(user.id);
    try {
      const token = getToken()!;
      const updated = await suspendUser(token, user.id);
      setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)));
    } catch {
      /* silent */
    } finally {
      setActionLoading(null);
    }
  };

  const handleActivate = async (user: User) => {
    setActionLoading(user.id);
    try {
      const token = getToken()!;
      const updated = await activateUser(token, user.id);
      setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)));
    } catch {
      /* silent */
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async (user: User) => {
    if (!confirm(`Delete user ${user.email}? This cannot be undone.`)) return;
    setActionLoading(user.id);
    try {
      const token = getToken()!;
      await deleteUser(token, user.id);
      setUsers((prev) => prev.filter((u) => u.id !== user.id));
    } catch {
      /* silent */
    } finally {
      setActionLoading(null);
    }
  };

  return (
    <DashboardLayout title="Users">
      {showModal && (
        <CreateUserModal onClose={() => setShowModal(false)} onCreated={handleCreated} />
      )}

      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            Organization Users
            <span className="tag tag-green" style={{ marginLeft: "8px" }}>
              Phase 1
            </span>
          </span>
          <button
            data-testid="add-user-btn"
            className="btn btn-primary"
            style={{ fontSize: "13px" }}
            onClick={() => setShowModal(true)}
          >
            + Add User
          </button>
        </div>

        {error && (
          <div
            style={{
              margin: "0 0 16px",
              padding: "12px 16px",
              background: "rgba(239,68,68,0.08)",
              border: "1px solid rgba(239,68,68,0.25)",
              borderRadius: "8px",
              color: "#ef4444",
              fontSize: "13px",
            }}
          >
            {error}
          </div>
        )}

        {loading ? (
          <div style={{ padding: "60px", textAlign: "center", color: "var(--muted)" }}>
            Loading users…
          </div>
        ) : users.length === 0 ? (
          <div
            data-testid="no-users-view"
            style={{ padding: "60px", textAlign: "center", color: "var(--muted)" }}
          >
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>👥</div>
            <h3>No users yet</h3>
            <p style={{ maxWidth: "350px", margin: "0 auto 20px" }}>
              Add members to your organization so they can access connected apps.
            </p>
            <button
              data-testid="add-user-empty-btn"
              className="btn btn-primary"
              onClick={() => setShowModal(true)}
            >
              + Add First User
            </button>
          </div>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table
              data-testid="user-table"
              style={{ width: "100%", borderCollapse: "collapse" }}
            >
              <thead>
                <tr style={{ borderBottom: "1px solid var(--border)" }}>
                  {["Email", "Display Name", "Status", "MFA", "Created", "Actions"].map(
                    (h) => (
                      <th
                        key={h}
                        style={{
                          padding: "10px 14px",
                          textAlign: "left",
                          fontSize: "11px",
                          fontWeight: 600,
                          textTransform: "uppercase",
                          letterSpacing: "0.05em",
                          color: "var(--muted)",
                        }}
                      >
                        {h}
                      </th>
                    )
                  )}
                </tr>
              </thead>
              <tbody data-testid="user-list">
                {users.map((user) => (
                  <tr
                    key={user.id}
                    data-testid={`user-row-${user.id}`}
                    style={{ borderBottom: "1px solid var(--border)" }}
                  >
                    <td style={{ padding: "12px 14px", fontSize: "13px" }}>
                      {user.email}
                    </td>
                    <td style={{ padding: "12px 14px", fontSize: "13px" }}>
                      {user.display_name || "—"}
                    </td>
                    <td style={{ padding: "12px 14px" }}>
                      <StatusBadge status={user.status} />
                    </td>
                    <td style={{ padding: "12px 14px", fontSize: "13px" }}>
                      {user.mfa_enabled ? (
                        <span className="tag tag-green">Enabled</span>
                      ) : (
                        <span className="tag">Off</span>
                      )}
                    </td>
                    <td
                      style={{
                        padding: "12px 14px",
                        fontSize: "12px",
                        color: "var(--muted)",
                      }}
                    >
                      {user.created_at ? formatDate(user.created_at) : "—"}
                    </td>
                    <td style={{ padding: "12px 14px" }}>
                      <div style={{ display: "flex", gap: "6px" }}>
                        {user.status === "active" ? (
                          <button
                            data-testid={`suspend-user-${user.id}`}
                            className="btn"
                            style={{ fontSize: "11px", padding: "4px 10px" }}
                            disabled={actionLoading === user.id}
                            onClick={() => handleSuspend(user)}
                          >
                            Suspend
                          </button>
                        ) : (
                          <button
                            data-testid={`activate-user-${user.id}`}
                            className="btn btn-primary"
                            style={{ fontSize: "11px", padding: "4px 10px" }}
                            disabled={actionLoading === user.id}
                            onClick={() => handleActivate(user)}
                          >
                            Activate
                          </button>
                        )}
                        <button
                          data-testid={`delete-user-${user.id}`}
                          className="btn"
                          style={{
                            fontSize: "11px",
                            padding: "4px 10px",
                            color: "#ef4444",
                            borderColor: "rgba(239,68,68,0.3)",
                          }}
                          disabled={actionLoading === user.id}
                          onClick={() => handleDelete(user)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
}
