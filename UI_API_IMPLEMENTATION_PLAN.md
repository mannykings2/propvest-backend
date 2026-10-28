# UI ↔ API Gap Audit & Implementation Plan

**Date:** 27 Oct 2026 · **Scope:** full cross-reference of the backend route
table (146 registrations in `internal/routes/v1/routes.go`) against every
frontend API call (~100 in `src/services/*`) and all 15 UI routes.

**Method:** enumerate both sides mechanically, diff both directions, then
classify each gap as *broken*, *should-have*, or *intentional*.

---

## Part 1 — UIs with no API behind them

| # | UI | Today | Backend exists? | Severity |
|---|---|---|---|---|
| 1 | **AuthModal registration consent** | Sends only name/email/phone/password — no `accept_terms` / `accept_privacy_policy` / `marketing_consent` | Yes — production default `REQUIRE_REGISTRATION_CONSENT=true` **rejects** the current payload; dev records **zero consent rows** | 🔴 P0 |
| 2 | Admin → Properties → **Create Listing** | *"In a real app this would POST to an API"* — closes and does nothing | 10 of 12 admin property endpoints unused (only list + publish wired) | 🟠 P1 |
| 3 | Admin → **Staff tab** | Local-only roster/role changes | No backend at all (documented demo) | 🟡 D-decision |
| 4 | Admin → **Roles tab** | Local permission matrix | No backend (documented demo) | 🟡 D-decision |
| 5 | Admin → Finance → **Payouts** | `PAYOUT_PROPS` demo distribution flow | No backend — investor rent distribution was never built | 🟡 D-decision |
| 6 | Admin → **Support Tickets block** | Hardcoded `SUPPORT_TICKETS` | Real `/admin/support/tickets` API exists (4 endpoints) | 🟠 P1 |
| 7 | **No user Privacy UI** | SPA calls none of the 9 user privacy endpoints | Backend complete (Phase 9) | 🟠 P1 |

Static by design (not gaps): marketing pages, `/legal`, Developer portal,
NotFound. Fallback demo data used only when an API errors — acceptable.

## Part 2 — Backend APIs with no UI

**Should have UI:**

1. **User privacy (9):** `GET/POST /me/consents`, `GET /me/marketing`,
   `POST/GET /me/exports`, `POST/GET/DELETE /me/account/delete`
2. **`GET /unsubscribe`**, **`GET /me/exports/download/:token`** — email link
   targets with no page; emails contain **no links at all** today
   (`NotifyWithEmail` renders `<p>body</p>`; `SendBulkMessage` appends no
   unsubscribe URL)
3. **Admin support (4):** `GET`, `GET/:id`, `POST/:id/replies`,
   `PATCH/:id/status` under `/admin/support/tickets`
4. **Admin properties (10):** create, update, delete, detail, images ×3,
   documents ×3
5. **Admin users (2):** `DELETE /admin/users/:id`, `PATCH /admin/users/:id`
6. **Admin investments (3):** `GET /admin/investments`,
   `GET /admin/investments/metrics`, `GET /admin/properties/:id/investments`

**Minor / defensible:** `GET /portfolio/summary` (computed client-side),
`GET /investments/:id`, `GET /admin/audit-logs/:id`, manual
`POST /admin/finance/settlements`.

**Intentionally UI-less:** `/health`, `/health/ready`, `/metrics`,
`/webhooks/*`, `/wallet/deposit/callback`.

**Reverse check:** every frontend call resolves to a real route — no broken
client calls.

---

## Implementation plan

### Phase A — P0: registration consent (S)
- [x] A1 AuthModal: required *Terms + Privacy Policy* checkbox, separate
      optional marketing checkbox; send `accept_terms`,
      `accept_privacy_policy`, `marketing_consent`
- [x] A2 Labels link to `/legal`; guard `handleRegister` when unchecked
- **Acceptance:** signup with `REQUIRE_REGISTRATION_CONSENT=true` succeeds and
  writes 3 `user_consents` rows; signup without the checkbox fails with a
  clear message; declining marketing never blocks signup.

### Phase B — user privacy UI (M)
- [ ] B1 `/privacy` page (entry: Profile + footer): consent timeline &
      toggles, marketing opt-in/out, export request/status/download, delete
      account (30-day countdown + cancel)
- [ ] B2 Email links: `FRONTEND_URL` config; render `download_path` and
      `GET /unsubscribe?token=` link in email HTML; `SendBulkMessage` appends
      the per-user unsubscribe URL
- [ ] B3 SPA landings `/unsubscribe` and export-download (fetch + save JSON)
- **Acceptance:** every NDPA right demonstrable from the browser; a suppressed
  address receives nothing; links in emails work.

### Phase C — admin: wire what exists (M)
- [ ] C1 Support tab → 4 real endpoints (queue, thread, reply, status)
- [ ] C2 Properties tab → create/edit/delete + image/document upload (kill the
      local-only path)
- [ ] C3 Users → delete (typed confirmation) + edit profile
- [ ] C4 Investments oversight → list + metrics + per-property investors
- **Acceptance:** no demo array rendered where a live endpoint exists; every
  mutation survives reload.

### Phase D — product decisions (founder)
- D1 Investor payouts: build the distribution engine **or** delete the demo UI
- D2 Staff/roles: build a staff model **or** badge the tabs as demo
- D3 `GET /portfolio/summary` / `GET /investments/:id`: wire or remove

**Order:** A → B → C1/C2 → C3/C4 → D. A is hours; B and C ~1–2 weeks total.