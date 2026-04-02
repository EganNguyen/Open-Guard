"use client";

import { useEffect, useState } from "react";
import DashboardLayout from "@/components/DashboardLayout";
import styles from "../dashboard.module.css";
import { getAuditEvents } from "@/lib/api";

export default function AuditPage() {
  const [events, setEvents] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadEvents();
  }, []);

  const loadEvents = async () => {
    try {
      setLoading(true);
      const token = localStorage.getItem("access_token");
      if (!token) throw new Error("No token");
      const resp = await getAuditEvents(token);
      setEvents(resp.data);
    } catch (err) {
      console.error("Failed to load audit events:", err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <DashboardLayout title="Audit log">
      <div className={styles.panel}>
        <div className={styles.panelHeader}>
          <span className={styles.panelTitle} data-testid="page-title">
            System Audit Events
          </span>
        </div>
        
        {loading ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>Loading...</div>
        ) : events.length === 0 ? (
          <div style={{ padding: "40px", textAlign: "center", color: "var(--muted)" }}>
            <div style={{ fontSize: "48px", marginBottom: "16px" }}>📜</div>
            <h3>No events found</h3>
            <p style={{ maxWidth: "400px", margin: "0 auto" }}>
              Events will appear here as you interact with the system.
            </p>
          </div>
        ) : (
          <table className={styles.auditTable}>
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Target</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody data-testid="audit-list">
              {events.map((event: any) => (
                <tr key={event.id}>
                  <td className={styles.timeCell}>{new Date(event.occurred_at).toLocaleString()}</td>
                  <td>
                    <div className={styles.actorCell}>
                      <span className={styles.miniAvatar} style={{ background: "var(--surface2)", border: "1px solid var(--border)" }}>
                        {event.actor_id?.substring(0, 2).toUpperCase() || "SY"}
                      </span>
                      {event.actor_email || event.actor_id}
                    </div>
                  </td>
                  <td>
                    <span className={styles.eventType}>{event.type}</span>
                  </td>
                  <td style={{ fontSize: "11px", color: "var(--muted)", fontFamily: "var(--mono)" }}>
                    {event.resource_id}
                  </td>
                  <td>
                    <span className={`tag tag-${event.status === 'success' ? 'green' : 'red'}`}>
                      {event.status || 'success'}
                    </span>
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
