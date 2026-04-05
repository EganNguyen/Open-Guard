# §20 — Appendix: Frontend Trade-offs & Decision Log

Mirrors BE spec Appendix A in format. Documents explicit frontend decisions so future engineers understand the why.

---

## 20.1 Trade-off Table

| Decision | Alternatives considered | Reason chosen |
|---|---|---|
| `httpOnly` cookies for access tokens (via NextAuth) | `localStorage` | `localStorage` is readable by any JavaScript on the page — XSS-accessible. `httpOnly` cookies are invisible to JavaScript; only the browser sends them. Aligns with BE spec §2.8 (SCIM org_id from token, not header). |
| SSE proxied through Next.js route handlers | Direct SSE from browser to backend | Direct SSE would require sending the access token as a URL param (visible in logs) or via a short-lived ticket. Route handler proxying keeps the token server-side; the browser only holds a session cookie. |
| TanStack Query for all server state | SWR, Zustand + fetch, React Context | TanStack Query provides: background refetch, stale-while-revalidate, infinite queries, optimistic updates, and devtools — all needed for this dashboard. SWR lacks infinite queries. Zustand + fetch is manual and error-prone. |
| Cursor reset on filter change | Maintain cursor across filter changes | Applying a new filter with an existing cursor from a previous result set would return results from an arbitrary position in the new result set — incorrect and confusing. Resetting cursor ensures page 1 of the new filtered results. |
| `nuqs` for URL filter state | `useState` / `useSearchParams` | `useState` filters are lost on page refresh. `nuqs` provides type-safe, serialized URL state that survives refresh, enables bookmarking, and supports browser back/forward navigation for filter changes. |
| Radix UI headless primitives | shadcn/ui, Chakra UI, MUI | shadcn/ui copies components into the project (no versioning issues). Radix headless gives full styling control. Chakra/MUI have strong opinions that clash with Tailwind-first approach. We want to own every pixel. |
| `ConfirmDialog` with `requireTyped` for destructive actions | Simple yes/no dialog | Single-click destructive actions on a security operations dashboard are a significant liability. Typing the resource name creates a cognitive speed bump that dramatically reduces accidental deletions. Matches GitHub's repository delete UX. |
| Audit stream capped at 200 events | Unbounded buffer | At 50,000 events/s (BE spec SLO), an unbounded buffer would cause rapid memory growth and DOM thrash. 200 events covers ~4ms of ingest — sufficient for real-time monitoring. Users who need history use the Historical mode with cursor pagination. |
| FIFO eviction for audit stream buffer | LIFO or priority eviction | FIFO retains the N most recent events (newest at top), which is what security analysts need when monitoring for recent suspicious activity. Matches the SDK circuit breaker eviction policy (BE spec §3.1). |
| No client-side org_id derivation | Read org_id from URL path | org_id in the URL is readable and editable by the user. A user could manually change it to another org's ID. `useOrg()` always reads from the JWT claim via the session — the server has already validated it. |
| Polling for compliance report status (3s interval) | WebSocket or SSE | Report generation takes 10–120s (BE spec SLO). The 3s polling interval is a good balance — responsive without excessive requests. SSE would need a persistent connection per in-progress job, which is wasteful for an infrequent operation. |
| Pre-signed S3 URL redirect for report download | Proxy download through Next.js | Proxying a multi-MB PDF through the Next.js server wastes server memory and adds latency. Redirecting to a pre-signed S3 URL shifts bandwidth to S3 (designed for this). The 1-hour TTL is sufficient for the UI interaction window. |
| `beforeunload` warning on API key reveal | Trust user to save key | The one-time key reveal is the highest-stakes moment in the connector registration flow. Accidentally navigating away means the key is permanently lost and must be rotated. The `beforeunload` warning is a last-resort safeguard — it cannot intercept all navigation (e.g., tab close) but catches accidental link clicks. |
| Server Components by default, `"use client"` only when needed | Client-first (SPA approach) | Server Components allow data to be fetched close to the source (server-side), reducing waterfall requests, eliminating client-side token exposure for initial data loads, and improving Time to First Byte. Client Components are used only when browser APIs or interactivity are essential. |
| `noUncheckedIndexedAccess: true` in tsconfig | Standard TypeScript strict mode | Array and object index access returns `T | undefined` by default in TypeScript, even with `strict: true`. Enabling `noUncheckedIndexedAccess` forces explicit null checks — prevents a class of runtime `undefined is not an object` bugs that standard strict mode misses. |
| Pagination cursor in URL (`?cursor=...`) | In-component state | Keeping the cursor in the URL allows browser back/forward navigation to work correctly for paginated list pages. A user who opens a detail page and returns lands on the same cursor position they left. In-component state is lost on navigation. |
| Global error boundary in `app/error.tsx` + per-section boundaries | Single global boundary only | A single global boundary turns any error into a full-page takeover. Per-section boundaries (e.g., wrapping the charts section separately from the table) allow the rest of the dashboard to remain functional when one data source fails — critical for a security operations tool where admins need to see *something* even during partial outages. |
| Form validation with Zod + React Hook Form | Yup + Formik, native HTML5 validation | Zod schemas are reusable on both client (validation) and can be adapted for API response parsing. React Hook Form avoids uncontrolled inputs and has minimal re-renders. Formik is heavier and more opinionated. HTML5 validation is not customizable enough for complex patterns like the scope selector or rule builder. |

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
