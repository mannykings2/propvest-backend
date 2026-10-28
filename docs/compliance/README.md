# PropVest Compliance Pack

**Status:** Phase 9 deliverable (9.8) — owner: Compliance Officer
**Effective:** Oct 2026 · **Next review:** every 6 months, or on any licence,
product or regulator change.

This directory is the **signed-off compliance pack** referenced by the Phase 5
go-live checklist. It is *documents, not code* — the code that produces its
evidence lives in Phases 6–9.

---

## Contents

| Document | Question it answers | Gate |
|---|---|---|
| [LICENCE_POSTURE.md](LICENCE_POSTURE.md) | Which licences do we need (CBN / SEC / NFIU), and which decisions block go-live? | **Blocks go-live** (L1, L2 open) |
| [NDIC_DISCLOSURE.md](NDIC_DISCLOSURE.md) | What may we legally say about insurance and safety of funds? | Blocks marketing release |
| [DATA_RETENTION_SCHEDULE.md](DATA_RETENTION_SCHEDULE.md) | What do we keep, for how long, and why does erasure keep the ledger? | Reviewed each release |
| [DPO_ASSESSMENT.md](DPO_ASSESSMENT.md) | Are we a DCMI, and do we need a DPO? | Blocks go-live |
| [BREACH_RUNBOOK.md](BREACH_RUNBOOK.md) | **Incident-breach notification procedure** — detection → severity → 72h NDPC clock → subject notification → evidence log | Drilled quarterly |
| [AML_CFT_POLICY.md](AML_CFT_POLICY.md) | AML/CFT policy (Phase 8) | Signed off |
| [STR_PROCEDURE.md](STR_PROCEDURE.md) | STR/CTR filing, 24h NFIU clock (Phase 8) | Signed off |
| [GOAML_REGISTRATION.md](GOAML_REGISTRATION.md) | goAML portal onboarding (Phase 8) | In progress |
| [STAFF_TRAINING.md](STAFF_TRAINING.md) | Compliance training cadence (Phase 8) | Scheduled |
| `breach_log.md` | One section per incident; T+0 stamps (created on first use) | — |

> The **incident-breach notification procedure** required by item 9.8 of the
> plan is [BREACH_RUNBOOK.md](BREACH_RUNBOOK.md) — it owns the 72-hour clock,
> the severity table, the subject-notification decision and the evidence log.

## Sign-off checklist

- [x] Consent capture, withdrawal, re-consent on policy bump (code + tests)
- [x] Access (export), erasure (deferred deletion), objection (suppression)
- [x] Marketing suppression enforced by the bulk sender
- [x] Admin Privacy UI: five tabs against live data
- [x] BVN/NIN encrypted at rest; decrypted only in the owner's own export
- [x] Breach runbook with 72h clock + tabletop
- [ ] **[COUNSEL]** Licence posture decisions L1 (wallet/MMO) and L2 (SEC/CIS)
- [ ] **[COUNSEL]** NDIC wording figures confirmed against ndic.gov.ng
- [ ] **[COUNSEL]** DCMI classification + DPO named and published
- [ ] goAML registration completed (Phase 8 carry-over)
- [ ] Compliance Officer signature: ______________________  date: __________

## How evidence is produced (for an auditor)

| Claim | Evidence |
|---|---|
| Consent is versioned and withdrawable | `user_consents` (append-only) + `policy_versions`; admin Privacy → Consent evidence |
| Erasure honours statutory retention | `account_deletions` + wallet/ledger rows surviving it; integration test `TestPrivacy_DeletionGraceAnonymisesButKeepsLedger` |
| Opt-outs are enforced | `marketing_suppressions`; bulk sender fails closed if the list is unreadable |
| Admin accountability | `audit_logs` (actor, IP, request_id) for KYC reveals, privacy writes, risk triage, finance actions |
| Breach readiness | [BREACH_RUNBOOK.md](BREACH_RUNBOOK.md) §7 tabletop record |
| PII at rest | `internal/utils/piienc` (AES-256-GCM), key outside the DB |