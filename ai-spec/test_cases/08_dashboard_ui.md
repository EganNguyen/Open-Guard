# Admin Dashboard (UI) Journeys

## 1. Overview
This module validates the user experience and visual components of the Angular-based Admin Dashboard.

## 2. Security Monitoring

### TC-UI-001: Threat Intelligence Dashboard
- **Journey:** Analyst logs in to view the global security posture.
- **Visual Verifications:**
  - "Threat Map" correctly displays geographic locations of `impossible_travel` alerts.
  - "Security Score" chart reflects real-time metrics from the Compliance service.
  - Alert list supports filtering by severity (High/Medium/Low) and status.

### TC-UI-002: Audit Log Explorer
- **Journey:** Compliance officer investigates a specific user's actions.
- **Verification:**
  - Real-time streaming of audit events via WebSockets.
  - Detailed view showing JSON payload and Hash Chain integrity status (Green/Red icon).

## 3. Configuration & Management

### TC-UI-003: Policy Builder (Visual CEL)
- **Journey:** Admin creates a complex ABAC policy without writing raw CEL.
- **Flow:**
  - Use UI dropdowns to select Attributes (Role, Resource, IP).
  - UI generates the correct CEL expression: `subject.role == 'admin' && resource.path.startsWith('/v1/secret')`.
  - Preview button validates the CEL syntax against the Policy Service.

### TC-UI-004: Connector Lifecycle
- **Journey:** Developer registers a new application.
- **Flow:**
  - Add Name, Redirect URIs, and Webhook URL.
  - UI displays the API Key **exactly once** in a "Secret Reveal" modal.
  - Verification of Webhook endpoint via a "Send Test Ping" button.

## 4. Compliance

### TC-UI-005: Compliance Report Management
- **Journey:** Download a SOC2 compliance report.
- **Flow:**
  - Click "Generate Report" -> Select Date Range.
  - UI shows progress bar (Pending -> Generating -> Ready).
  - Download button serves the RSA-signed PDF.
