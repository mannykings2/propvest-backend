# Developer Portal: Frontend/Backend Parity Plan

## Current state (verified against code)

The portal at `propvest-app/src/features/developer/DeveloperPortal.jsx` is a demo, not a connected developer workflow:

- Overview imports `DEV_PROPS` from static `constants/data.js`; organization name, verified status, pooled amounts, investor counts, rent, distributions, documents and subscribers are sample values.
- Listing wizard does not call an API. Document upload steps are booleans toggled by clicking cards; no file is selected or sent. Submit previously showed a fake "Submission Received" screen with a random `DEV-####` reference; it now states plainly that nothing was saved or submitted, and the overview carries a standing demo-data notice. Both are placeholders to remove in Phase 4.
- Property detail and subscribers/documents tabs read hard-coded sample values (fake investor names, rent, distributions, document dates).
- Access control is sign-in only: `/developer` is wrapped in `RequireAuth` but never checks `role === "developer"`, so any investor can open the portal and flip the portal switcher to developer mode.

Backend status:

- `developer` is a valid user role, but the route registration has no `/developer` or `/developers` group and no developer-specific endpoints.
- Public `GET /properties` and `GET /properties/:id` return published opportunities.
- Property CRUD and image/document management are under `/admin/properties`, protected by admin role. They cannot safely serve the developer portal.
- Property records contain `CreatedBy` / `UpdatedBy` audit fields but no developer owner/organization relationship; create currently receives the acting admin ID. There is no way to scope a listing to its developer.
- Off-plan read/subscribe routes serve investor-facing off-plan projects; they are not developer management routes.
- Existing architecture docs describe developer-owned listings, supporting documents, funding tracking and developer verification, while the API mapping doc labels the portal "Mostly missing". The implementation matches that gap, not the intended workflow.

## Target principles

1. A developer can only read or change records owned by their authenticated `user_id` (derive it from the auth context; never accept an owner ID from the browser).
2. Developers can create/edit drafts and submit for review, but cannot self-approve, publish, alter funding counters, or view personally identifying investor data.
3. Admin review, status changes and document decisions remain separately permissioned and auditable.
4. Only show real numbers returned by the API. Distinguish unavailable values from zero, empty lists and successful submissions.
5. Keep monetary values in integer kobo in persistence/API calculations; format Naira only at the UI boundary.

## Phased implementation plan

### Phase 1 — Ownership and lifecycle foundation

- Add a nullable/backfilled `developer_id` (or explicit developer-organization FK) to properties and a developer profile/organization record with verification state. Decide data migration for existing admin-created listings: leave as platform-owned or assign through an audited admin process; do not guess ownership.
- Define lifecycle statuses such as `draft`, `submitted`, `in_review`, `changes_requested`, `approved`, `active`, `funded`, `rejected`, `completed`. Add transition validation and status history; developers must not write status directly.
- Add migration rollback, model/repository tests, and audit actor/reason on ownership and lifecycle changes.

### Phase 2 — Developer API and authorization

Add authenticated routes protected by `RequireRole("developer")` and ownership checks in repository/service queries:

- `GET /developer/profile` — organization + verification state.
- `GET /developer/dashboard` — owned listing counts by status, aggregate funds raised, investor counts (aggregates only), outstanding review actions; no fabricated values.
- `GET /developer/properties` and `GET /developer/properties/:id` — owned properties only, paginated, with lifecycle/history and safe financial summary.
- `POST /developer/properties` — create draft, setting developer owner server-side.
- `PATCH /developer/properties/:id` — edit only while draft/changes-requested; validate fields and reject edits after approval/funding except through controlled change request.
- `POST /developer/properties/:id/submit` — submit for review; server validates required data/docs and returns persisted ID + status + request ID.
- `POST /developer/properties/:id/documents` and `GET/DELETE .../documents/:documentId` — allowlisted file types/size, private storage, malware scanning, authorization, immutable review evidence. Reuse existing storage mechanisms only after their access controls are confirmed.
- No endpoint returns subscriber names or KYC/identity fields. If operationally necessary later, expose anonymized aggregate metrics only.

Use stable JSON DTOs, consistent `request_id`, validation errors, pagination and idempotency for submit. Verify developer A cannot access developer B's property/document via IDs.

### Phase 3 — Admin review and verification

- Add admin queue/filter/detail endpoints for developer verification and submitted properties.
- Admin actions: approve/reject/request changes with required reason; document-level accept/reject; set review SLA/assignment; publish only after approval.
- Ensure every decision emits an audit record and notification, and keep review status separate from public `active` status.
- Include escalation/withdrawal of approval and safe change-request path after a listing has investor activity.

### Phase 4 — Replace demo portal with real API states

- Replace `DEV_PROPS` imports with `developerService` calls for profile, dashboard, listings, property details and documents.
- Implement loading, empty, forbidden/unverified, offline/error+retry, draft, review-pending, changes-requested, approved and rejected states.
- Make the listing wizard save a draft and resume it; use actual multipart file selection/upload with progress, per-document status, remove/replace action, and server-provided submission reference.
- Show real review history/timestamps and actionable reasons for returned changes.
- Remove hard-coded investor names, rent/distribution values, status labels, estimated review promises, fake document upload toggles and random references.
- Display only aggregate investor/funding counts. Clearly label estimated vs realized/distributed amounts; don't claim rent guarantees unless supported by product terms and data.

### Phase 5 — Verification and operations

- Unit/API tests for validation and lifecycle transitions; integration tests for ownership isolation, role denial, document access, retry/idempotency, and no PII exposure.
- Frontend tests for loading/empty/error/retry and submission success only after server confirmation.
- Update OpenAPI/API mapping, data model docs, developer onboarding, privacy notice and admin SOP.
- Add monitoring for submission failures, queue age, review SLA, document scanning and unauthorized access. Roll out behind a feature flag to a test developer account, then enable after data migration and operational review.

## Additional developer-portal features (after core parity)

1. **Submission checklist and readiness score:** specific missing fields/docs and why each is needed.
2. **Review timeline and change requests:** a durable conversation thread tied to a submission, with clear owner and due date.
3. **Draft autosave and version history:** recover interrupted work; show what changed between submitted versions.
4. **Funding and milestone reporting:** aggregate committed/settled amounts, milestones, completion evidence, and investor update history; separate pledged, paid, settled and distributed money.
5. **Document expiry/reminders:** alert on expiring approvals/insurance/title evidence, with audit history.
6. **Notifications and exports:** email/in-app events for status, requested changes and funding milestones; downloadable owned-property reports with no investor PII.
7. **Team/organization roles:** only when multiple staff per developer company are needed; model explicit organization membership and least-privilege roles rather than sharing logins.

## Delivery order

Start with Phase 1 ownership and lifecycle decisions, then Phase 2 secure API, Phase 3 admin review, Phase 4 UI replacement, and Phase 5 test/rollout. Do not wire the current mock subscriber table to investor data: it exposes personal names and stakes without a demonstrated privacy/authorization basis.
