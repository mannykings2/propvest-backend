# PropVest AML/CFT Policy

**Status:** Signed-off framework — owner: Compliance Officer
**Effective:** Phase 8 (Oct 2026)
**Reviewed:** at least annually, or on any regulatory change
**Related:** [STR Procedure](STR_PROCEDURE.md) · [goAML Registration](GOAML_REGISTRATION.md) · [Staff Training](STAFF_TRAINING.md)

---

## 1. Purpose and scope

This policy states how PropVest Investments Ltd prevents its platform from being
used for money laundering, terrorism financing and fraud. It applies to every
employee, contractor and every system that moves money on the platform —
deposits, withdrawals, investments, distributions and settlement.

Regulatory anchors:

| Body | Instrument | What it means here |
|---|---|---|
| CBN | AML/CFT/CPF Regulations 2022 | Risk-based programme, CDD, STR/CTR duties |
| NFIU | MLPPA 2022 | STR within **24h** of determination; CTR (cash) above ₦5M/₦10M within **7 days** |
| NDPC | NDPA 2023 | Personal data (BVN/NIN) processed lawfully, encrypted at rest, retained only as long as required |

## 2. Controls that are live in the platform

**Identity (CDD).**
- BVN + NIN + document capture on every account (Phase 6).
- Both numbers stored as AES-256-GCM ciphertext; plaintext exists only in
  process memory and only inside the admin detail view, which itself writes an
  audit row (`user.kyc_viewed`).
- KYC approval requires a recorded sanctions/PEP screening marked *clear*; a
  *hit* blocks approval, freezes the account and escalates automatically.

**Screening.**
- Manual sanctions/PEP check against public lists (OFAC SDN, UN, EU, NFIU
  watchlist) at KYC approval and every **30 days** thereafter for active users
  (`GET /admin/risk/rescreen` reminder). No third-party provider is integrated
  by design decision; the attestation (source, result, who, when) is recorded
  in `screening_results`.

**Transaction monitoring (Phase 8).**
- *Blocking* rules refuse the transaction synchronously: withdrawal velocity
  (>5/24h), new-account high-value withdrawal (first 7 days, ≥ ₦1M), and any
  withdrawal by a flagged account.
- *Monitoring* rules file alerts for triage: deposit velocity, cumulative
  deposit cap, structuring near the reporting threshold, round-amount cycling,
  rapid in-and-out, duplicate account funding, KYC-name vs payout-account
  mismatch.
- Every rule is toggleable with editable thresholds from the admin Risk panel;
  every toggle and threshold edit is written to the audit log.

**Pre-existing hard controls (Phases 1–7).**
Webhook signature verification, provider-side payment verification,
`SELECT … FOR UPDATE` wallet locks, idempotency keys, account-name resolution
before transfer, withdrawal rate limiting, admin suspension with session
revocation, and a full audit trail on every admin action.

**Money movement.**
Double-entry ledger with an append-only entries table; user funds ring-fenced
in custody accounts (2000-series); the **only** legal path from user money to
platform money is the audited fee sweep; nightly reconciliation proves the
books.

## 3. Risk-based approach

| Risk factor | Treatment |
|---|---|
| PEP / sanctions hit | Freeze, escalate, STR — never auto-clear |
| New account (< 7 days) | High-value withdrawals blocked outright |
| High velocity | Blocked above 5 withdrawals/24h; alerts for deposit velocity |
| Structuring | Alert at 3+ transactions in 90–99% of the reporting threshold in 7 days |
| Payout account ≠ KYC name | Alert per withdrawal for analyst review |
| Shared funding source | Alert when one funding source moneyed 2+ wallets in 7 days |

## 4. Roles

- **Compliance Officer** — owns this policy, clears alerts, files STR/CTR,
  runs the goAML portal.
- **Admin (PropVest staff)** — triages alerts, records screening attestations,
  flags/unflags accounts. Cannot delete alerts or audit rows.
- **Engineers** — cannot change thresholds without an audited admin toggle;
  rule parameters live in the database, not in code.

## 5. Records

- Audit logs: immutable application trail (actor, action, target, reason, IP,
  request id) — retained per the retention schedule.
- `risk_alerts`, `screening_results`, `str_reports`, `ctr_reports`: the
  compliance record for monitoring and reporting; retained for at least
  **5 years** after relationship end (MLPPA 2022).

## 6. Breach of this policy

Any deviation (approving KYC without screening, clearing an alert without
notes, disabling a monitoring rule without Compliance sign-off) is treated as
a disciplinary matter and is itself visible in the audit log.

---

*Owner signature: ______________________  Date: ____________*