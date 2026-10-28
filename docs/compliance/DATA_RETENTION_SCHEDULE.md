# PropVest Data-Retention Schedule

**Status:** Draft — owner: DPO / Compliance Officer
**Effective:** Phase 9 (Oct 2026)
**Principle:** NDPA 2023 storage limitation — keep personal data only as long
as the purpose requires **or** as statute requires. Where the two conflict,
**statute wins and is written down here** (that is the legal basis on which
`ExecuteDueDeletions` retains financial rows after an erasure).

---

## 1. The schedule

| # | Data | Where it lives | Retention | Basis / trigger for deletion |
|---|---|---|---|---|
| R1 | Profile identity (name, email, phone, address, occupation) | `users` | Life of account, then **anonymised** | Erasure request after 30-day grace (`ExecuteDueDeletions`) replaces identity with non-routable placeholders |
| R2 | Password hash, sessions | `users.password_hash`, `refresh_tokens` | Session lifetime (rotating refresh) | Deleted at erasure; tokens expire on their own |
| R3 | **BVN / NIN** (AES-256-GCM at rest) | `kyc_submissions.bvn/nin` | Until erasure, **or 5 years after the relationship ends if statute requires** | Scrubbed to empty string at erasure; the submission *row* (status history) is kept as evidence |
| R4 | KYC evidence documents | `kyc_submissions.evidence` | Until review completes + 5 years (AML) | `evidence = NULL` at erasure |
| R5 | Wallet transactions, ledger entries, payments, investments, settlements | `wallet_transactions`, `ledger_entries`, `payments`, `investments`, `settlement_*` | **5 years** after the last movement (MLPPA 2022 / tax) | **Never deleted by erasure** — statutory exception; nightly reconciliation depends on it |
| R6 | Consent evidence | `user_consents` | **Life of account + 6 years** after withdrawal | Append-only; must outlive the consent it proves |
| R7 | Marketing suppression list | `marketing_suppressions` | **Indefinitely** until the person re-consents | Deleting it would re-permit marketing that was objected to |
| R8 | Data-export bundles | `data_export_jobs.bundle` (JSONB) | **24 hours** | Worker sweep wipes the bundle on expiry; only the job row remains |
| R9 | Erasure request history | `account_deletions` | 6 years after execution | Evidence that a request was served and honoured |
| R10 | Audit logs (incl. admin KYC reveals, privacy actions) | `audit_logs` | **5 years** (redacted fields only — allowlist already strips secrets) | Never deleted inside the window |
| R11 | Risk alerts / screening / STR & CTR filings | `risk_alerts`, `screening_results`, `str_reports`, `ctr_reports` | STR/CTR: **5 years** from filing (NFIU); alerts: 3 years | Legal hold overrides everything |
| R12 | Support tickets + replies | `support_tickets`, `support_replies` | 2 years after closure | Erasure request anonymises the author if within window |
| R13 | Notification history | `notifications` | 13 months | Rolling purge in the worker |
| R14 | Email/SMS dispatch logs, webhook raw payloads | queue + `payments` raw payload column | 12 months / with R5 respectively | Provider-evidence window |
| R15 | Deletion requests, breach log, tabletop records | `docs/compliance/`, `account_deletions` | 6 years | Compliance evidence |

## 2. The erasure exception, stated plainly

When a user's account is erased (Phase 9.3):

- **Deleted / anonymised:** name → "Deleted User", email →
  `deleted+<id>@deleted.invalid`, phone randomised, password hash replaced,
  address/occupation/employer/avatar cleared, BVN/NIN ciphertext and KYC
  documents scrubbed, sessions revoked.
- **Retained:** financial rows (R5), consent evidence (R6), suppression rows
  (R7), audit trail (R10), risk/STR records (R11).

This is the NDPA erasure exception for compliance with a legal obligation —
the AML/tax record-keeping duty. It is the reason `mirrorMismatches` stays
green after an erasure: money conservation proof is independent of identity.

## 3. Legal holds

A hold (litigation, regulator enquiry, fraud investigation) suspends every
scheduled deletion in this table for the affected data. The Compliance
Officer raises holds in writing; the worker sweeps are the only deletion
mechanism, so a hold is implemented by *not* executing the relevant sweep
and recording the reason in `docs/compliance/breach_log.md`.

## 4. Operational hooks (already built)

| Retention | Mechanism |
|---|---|
| R2, R3, R4, erasure | `cmd/worker/privacy_sweeper.go` → `ExecuteDueDeletions` |
| R8 (24h export bundles) | same sweeper → `ExpireExportBundles` |
| R13 (notifications) | existing notification purge job |
| R5 mirror check | nightly reconciliation (`ledger_trust`) |

## 5. Open items

- [ ] **[COUNSEL]** Confirm the 5-year AML/tax windows against the MLPPA 2022
      and FIRS record-keeping rules for a *non-bank* platform.
- [ ] Confirm whether off-plan subscriber records inherit R5 or R12.
- [ ] Review this table whenever a new table ships (it is checked in the
      Phase 9.8 sign-off).