# §20 — Appendix: Frontend Trade-offs & Decision Log

Mirrors BE spec Appendix A in format. Documents explicit frontend decisions so future engineers understand the why.

---

## 20.1 Trade-off Table

| Decision | Alternatives considered | Reason chosen |
|---|---|---|
| Secure cookie-based auth | `localStorage` | `localStorage` is XSS-accessible. We use secure cookies for the session. Aligns with BE spec §2.8. |
| Direct SSE with Query Param Auth | Proxying through server | Proxying adds latency. Modern API gateways allow authorizing SSE via tokens. The `SseService` manages the connection life-cycle. |
| Angular Signals for State | TanStack Query, Zustand, RxJS-only | Signals provide granular reactivity and are now built into Angular. They eliminate the need for external state management libraries like Zustand or TanStack Query. |
| Angular Router for URL state | `nuqs`, manual `useState` | The Angular Router's `queryParams` and `RouterLink` provide a first-class way to manage serialized state that survives refreshes and supports back/forward navigation. |
| Vanilla CSS + Tailwind | component libraries | We want full control over the UI. Tailwind provides a utility-first approach that integrates seamlessly with Angular components. |
| `ConfirmDialog` with `requireTyped` for destructive actions | Simple yes/no dialog | Single-click destructive actions on a security operations dashboard are a significant liability. Typing the resource name creates a cognitive speed bump that dramatically reduces accidental deletions. Matches GitHub's repository delete UX. |
| Audit stream capped at 200 events | Unbounded buffer | At 50,000 events/s (BE spec SLO), an unbounded buffer would cause rapid memory growth and DOM thrash. 200 events covers ~4ms of ingest — sufficient for real-time monitoring. Users who need history use the Historical mode with cursor pagination. |
| `noOrgContext` guard | Read org_id from URL path | `org_id` in the URL is user-editable. The `AuthService` always reads the `org_id` claim from the decrypted JWT, ensuring a secure boundary. |
| Polling for compliance reports | WebSocket or SSE | 3s polling is a good balance for 10-120s operations. It avoids persistent connection overhead for infrequent tasks. |
| Pre-signed S3 URL redirect | Proxying downloads | Shifting bandwidth to S3 reduces load on the app server. The 1-hour TTL balance security and UX. |
| `beforeunload` on API key reveal | Trust user | Last-resort safeguard for the one-time key reveal. Prevents accidental loss of keys. |
| Modern SPA (Angular) | Server Components (Next.js) | Angular's Standalone components and Signals provide a superior developer experience and highly responsive, reactive UI. We leverage SSR for initial load performance while maintaining SPA richness. |
| `noUncheckedIndexedAccess: true` | Standard strict mode | Forces explicit null checks for array/object access, preventing common runtime crashes. |
| Global error handling | Single global boundary | Angular's `ErrorHandler` combined with per-feature error states ensures high availability of the dashboard during partial failures. |
| Angular Reactive Forms | Template-driven, raw inputs | Reactive forms provide a robust, type-safe API for complex validations and asynchronous checks. |

---

## 20.2 Out of Scope for v1

These features are documented as future work and must not be implemented until the relevant BE spec phase is completed:

| Feature | Dependency |
|---|---|
| Light mode | Design system Phase 2 |
| Multi-language / i18n | Not planned for v1 |
| Mobile-native app | Future product decision |
| Schema-tier / shard-tier org isolation UI | BE spec §2.3: only shared tier in v1 |
| Active-active multi-region org switcher | BE spec §18.6: active-passive only in v1 |
| SAML SP metadata editor UI | Current: IdP metadata URL is an env var |
| Connector event schema explorer | Future connector developer tooling |
| AI-assisted policy rule suggestions | Future ML feature |
| Custom DLP regex library sharing across orgs | DLP Phase 2 |
| Real-time compliance score updates | Currently: 10-minute polling |
| PDF report annotations / comments | Future collaboration feature |

---

## 20.3 Known Limitations

| Limitation | Explanation |
|---|---|
| `beforeunload` does not fire on tab close in all browsers | Safari omits the dialog on tab close. The API key is still lost if the user closes the tab without acknowledging. Mitigation: the warning text is prominent. |
| SSE reconnects can cause duplicate events | `EventSource` auto-reconnects on error. If an event was received just before disconnect and replayed after reconnect, it appears twice in the live buffer. The audit DB deduplicates on `event_id`; the frontend buffer does not. Duplicates are visual only. |
| Pre-signed S3 URL expires in 1 hour | If a user opens the compliance report page and leaves the tab open for >1 hour, the download link expires. They must refresh the page to get a new URL. This is a deliberate security trade-off (BE spec §14.3). |
| No optimistic update for complex mutations | Optimistic updates are only applied for simple status toggles (suspend/activate). Multi-field form submissions (policy create, DLP policy update) invalidate the query on success and re-fetch — a brief loading state is visible. |
| Global search latency | The global search debounces at 300ms and fans out to 5 API endpoints in parallel. On a slow connection, results may appear with noticeable delay. Search results are not cached between sessions. |
