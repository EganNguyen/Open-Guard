import React from 'react';

export default function ConnectorsPage() {
  return (
    <div style={{ padding: '2rem', fontFamily: 'system-ui, sans-serif' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '2rem' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '24px' }}>Connected Apps</h1>
          <p style={{ color: '#666', margin: '4px 0 0 0' }}>Manage integrations and API access for your organization.</p>
        </div>
        <button style={{ backgroundColor: '#000', color: '#fff', padding: '10px 20px', borderRadius: '6px', border: 'none', cursor: 'pointer', fontWeight: 600 }}>
          Register App
        </button>
      </header>

      <div style={{ border: '1px solid #eaeaea', borderRadius: '8px', overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
          <thead>
            <tr style={{ backgroundColor: '#fafafa', borderBottom: '1px solid #eaeaea' }}>
              <th style={{ padding: '16px', fontWeight: 500, color: '#444' }}>App Name</th>
              <th style={{ padding: '16px', fontWeight: 500, color: '#444' }}>Status</th>
              <th style={{ padding: '16px', fontWeight: 500, color: '#444' }}>Scopes</th>
              <th style={{ padding: '16px', fontWeight: 500, color: '#444' }}>Created Date</th>
              <th style={{ padding: '16px', fontWeight: 500, color: '#444' }}>Event Volume (30d)</th>
            </tr>
          </thead>
          <tbody>
            <tr style={{ borderBottom: '1px solid #eaeaea' }}>
              <td style={{ padding: '16px', fontWeight: 500 }}>Acme HR Sync</td>
              <td style={{ padding: '16px' }}>
                <span style={{ backgroundColor: '#e6f4ea', color: '#137333', padding: '4px 8px', borderRadius: '4px', fontSize: '12px', fontWeight: 600 }}>Active</span>
              </td>
              <td style={{ padding: '16px' }}>
                <span style={{ backgroundColor: '#f1f3f4', color: '#3c4043', padding: '4px 8px', borderRadius: '4px', fontSize: '12px', marginRight: '4px' }}>scim:write</span>
                <span style={{ backgroundColor: '#f1f3f4', color: '#3c4043', padding: '4px 8px', borderRadius: '4px', fontSize: '12px' }}>events:write</span>
              </td>
              <td style={{ padding: '16px', color: '#666' }}>Apr 6, 2026</td>
              <td style={{ padding: '16px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span>45,210</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      {/* Placeholder for Registration Modal */}
    </div>
  );
}
