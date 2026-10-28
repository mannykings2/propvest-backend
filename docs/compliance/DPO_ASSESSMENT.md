# DPO / DCMI Assessment (NDPA 2023 + GAID 2025)

**Status:** Draft assessment — owner: Founders, reviewed with the
DPO-designate. **[COUNSEL]** confirm before filing anything with the NDPC.
**Effective:** Phase 9 (Oct 2026)

---

## 1. What the law actually asks

The NDPA 2023 does **not** require every organisation to appoint a Data
Protection Officer. A DPO is mandatory for a **data controller or processor of
major importance (DCMI)** — broadly, large-scale or systematic/systematic
monitoring processing that carries high risk to individuals.¹ The **GAID
2025** then adds the operational detail: registration/audit and
compliance-reporting duties, **semi-annual internal DPO reporting**, and an
NDPC credential-assessment process for DPOs.² The old NDPR "250+ employees"
rule is not the test — do not rely on it.

## 2. PropVest's processing profile

| Factor | Our position | Pushes toward DCMI? |
|---|---|---|
| Special-category data | BVN + NIN + identity documents + biometric-adjacent evidence | **Strongly yes** |
| Volume of data subjects | Fintech consumer scale is by definition large | Yes |
| Systematic monitoring | Continuous transaction monitoring, AML velocity rules, risk scoring, audit trail | **Yes** |
| Financial + identity data at scale | Wallet balances, bank accounts, full transaction history | **Yes** |
| Cross-border | Email/SMS providers, payment processor; **no** offshore analytics by default | Moderate — document safeguards |
| Employees | Small team | No |

**Assessment: treat PropVest as a DCMI.** Processing BVN/NIN at consumer scale
with automated monitoring is precisely the profile the category exists for.

## 3. Recommendations (and what's already built)

1. **Appoint a DPO** — even a fractional/external one — and publish contact
   details (`dpo@…`) in the privacy policy and on `/legal`. *Owner: founders.*
2. **Register/file with the NDPC** as required for DCMI status under GAID.
   *Owner: DPO-designate — [COUNSEL] to confirm the current filing route.*
3. **Semi-annual internal report** (GAID): data-subject requests served,
   consents recorded/withdrawn, breaches, retention actions, policy version
   changes. The data for this already exists in the admin Privacy tab and
   audit log; the DPO turns it into a written report twice a year.
4. **Credential assessment** for the DPO (GAID process) — [COUNSEL] confirm
   the operative programme.
5. **Keep the record of processing activities current** — this schedule and
   the architecture docs are the source; update on every new table.

## 4. Evidence that already exists in the codebase

| GAID expectation | Where it is satisfied |
|---|---|
| Implementable data-subject rights | `GET/POST /me/consents`, `POST /me/exports` (async bundle, 24h link), `DELETE /me/account` (30-day grace → anonymise, ledger retained) |
| Consent: informed, versioned, **withdrawable** | `policy_versions` + append-only `user_consents`; withdrawal writes a new row; policy bump forces re-consent |
| Objection to direct marketing | Unsubscribe token + `marketing_suppressions`, enforced by the bulk sender |
| Purpose separation (marketing ≠ contract) | Separate marketing consent; declining it never blocks signup |
| PII protection at rest | BVN/NIN AES-256-GCM (`piienc`), separate key, decrypted only for the owner's own export |
| Breach response | [BREACH_RUNBOOK.md](BREACH_RUNBOOK.md) — 72h NDPC clock |
| Accountability / audit | `audit_logs` with actor, IP, request_id; admin actions audited incl. KYC reveal and every privacy write |
| Storage limitation | [DATA_RETENTION_SCHEDULE.md](DATA_RETENTION_SCHEDULE.md) + worker sweeps |

## 5. Open items

- [ ] **[COUNSEL]** Confirm DCMI classification and the exact GAID filing/
      registration steps in force.
- [ ] Name the DPO and publish the contact route.
- [ ] Set the semi-annual report calendar (June / December) and add a line to
      `docs/compliance/breach_log.md`-style evidence for each cycle.

## Sources

1. FPF, "Nigeria's New Data Protection Act Explained":
   https://fpf.org/blog/nigerias-new-data-protection-act-explained/
2. Legal 500, review of the NDPC GAID:
   https://www.legal500.com/intelligence/nigeria/privacy/compliance-made-easy-a-review-of-the-nigeria-data-protection-commission-ndpc-general-application-and-implementation-directive-gaid
3. DLA Piper Nigeria data-protection overview:
   https://www.dlapiperdataprotection.com/index.html?t=law&c=NG