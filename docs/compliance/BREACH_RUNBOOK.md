# PropVest Data-Breach Runbook (NDPA 2023 — 72-hour NDPC clock)

**Status:** Signed-off procedure — owner: Compliance Officer / DPO (see [DPO_ASSESSMENT.md](DPO_ASSESSMENT.md))
**Effective:** Phase 9 (Oct 2026)
**Clock:** notify the **NDPC within 72 hours** of becoming aware of a breach
(NDPA 2023 s.40); notify data subjects **without undue delay** where the risk
to their rights and freedoms is high. "Aware" means the controller has a
reasonable degree of certainty that a security incident compromising personal
data occurred — not the moment the full investigation finishes.

---

## 1. Detection — where an incident comes from

| Source | What it looks like | First action |
|---|---|---|
| Platform audit trail | `audit_logs` rows for unexpected admin reads (`user.kyc_viewed`, bulk exports), failed auth spikes | Open the admin **Compliance** tab, filter by actor + time window |
| Risk engine | alerts from the AML rules (velocity, structuring, new-account high-value) | Triage in the **Risk** tab; a *fraud* alert that implies account takeover of many users is also a breach signal |
| Wallet / ledger | reconciliation failure (`nightly trust check` red), unexplained balance delta | Freeze withdrawals first, then investigate (see [AUDIT_05_RECONCILIATION.md](../../AUDIT_05_RECONCILIATION.md)) |
| Provider / infra | Paystack or SMTP incident, lost laptop, leaked DB credential, S3/R2 misconfig | Treat as potential breach until disproved |
| External | researcher report, customer complaint, press, regulator enquiry | Log receipt time — this may be your "aware" timestamp |

**Anything that could expose BVN/NIN, identity documents, bank details or
transaction history starts the clock.** When unsure, start the clock; a
precautionary notification that turns out unnecessary is cheap, a late one is
not.

## 2. Severity classification

| Level | Meaning | Examples | Notification path |
|---|---|---|---|
| **S1 — Critical** | Mass PII exposure or money at risk | DB dump leaked; BVN/NIN table exfiltrated; takeover of many accounts | NDPC ≤ 72h **and** affected data subjects without undue delay |
| **S2 — Major** | Scoped PII exposure, clear risk to individuals | One admin account compromised, KYC documents read; 100–1,000 records exposed | NDPC ≤ 72h; subjects per risk assessment |
| **S3 — Minor** | Limited or unlikely-to-harm exposure | Single record visible to the wrong user; failed attempt with no data read | Internal log; notify NDPC only if risk accumulates |
| **S4 — None** | Incident with no personal-data compromise | Dry-run, spam, no data accessed | Internal log only |

Severity is decided by the Compliance Officer (S3/S4) or with the DPO
(S1/S2) **within the first 4 hours**. Record the decision and its rationale —
it is exhibit A if the NDPC later asks why you notified (or did not).

## 3. The 72-hour NDPC clock

```
T+0h   Awareness: incident confirmed (or reasonably suspected). Stamp it.
T+4h   Severity set; containment started; evidence preserved.
T+24h  Facts assembled: what data, whose data, how many, cause, containment.
T+48h  Notification drafted (Section 4 form); DPO + management sign-off.
T+72h  ✅ Filed with the NDPC — portal/email, proof of submission saved.
```

Rules that keep the clock honest:

1. **T+0 is recorded in writing** (`docs/compliance/breach_log.md`, see §6) —
   not remembered.
2. The clock does **not** pause for an incomplete investigation. File what is
   known at T+72h and supplement afterwards.
3. A breach with a *risk* to individuals must be notified even if the cause is
   still unknown — say "under investigation".
4. Notifications are logged with an audit row (`privacy.breach_notified`) the
   same day they are sent.

## 4. What the NDPC notification contains

1. **Nature of the breach** — categories and approximate number of data
   subjects and records (e.g. "≈120 users, BVN + NIN + name").
2. **DPO / contact point** — name, email, phone of the responsible person.
3. **Likely consequences** — identity theft, fraud, financial loss.
4. **Measures taken** — containment, reset, enhanced monitoring, and the
   measures proposed to mitigate effects.
5. **Timeline** — when it happened, when discovered, when contained.

Where notification to data subjects is required, use the same content in
plain language: what happened, what data, what we are doing, what the
individual should do (password change, BVN/NIN misuse watch), and a contact
route. No jargon, no blame-shifting.

## 5. Subject-notification decision

Notify data subjects **without undue delay** when the risk to their rights
and freedoms is high. Presume high risk for: BVN/NIN or identity-document
exposure, financial credentials or wallet takeover, KYC document leak, and
any confirmed unauthorised withdrawals. Presume low risk for encrypted data
with a strong key (e.g. PII at rest under AES-256-GCM where the key was not
exposed) or incidents with no realistic misuse path — document the reasoning
either way.

Channel: email (primary) + in-app notification; SMS if the email address may
be compromised. Keep the wording in §4.

## 6. Evidence log (the exhibit)

Maintain `docs/compliance/breach_log.md` — one section per incident:

```markdown
## BR-YYYY-NNN — <one-line summary>
- T+0 (awareness): <timestamp, who, how it was discovered>
- Severity: S__ — decided by <name> at <timestamp>, rationale: …
- Data involved: <categories, record count, encryption state>
- Root cause: <known / under investigation>
- Containment: <actions + timestamps>
- NDPC notified: <timestamp> — reference: … (proof: …)
- Subjects notified: <count / n/a> — <channels> — <timestamp>
- Post-incident review: <date, actions, owners, due dates>
- Audit rows: <ids>
```

## 7. Tabletop walkthrough (run quarterly)

Scenario: **a read-replica credential is found in a public repository, valid
for 6 days; access logs show SELECTs against `kyc_submissions`.**

| Step | Expected action | Pass criteria |
|---|---|---|
| 1. Detect | Engineer notices the credential in a secret scan | T+0 stamped in `breach_log.md` within 1 hour of discovery |
| 2. Contain | Rotate credential, revoke sessions (`admin: logout-all`), freeze withdrawals if wallet data is implicated | Access blocked within 4h; evidence (logs) preserved before rotation |
| 3. Scope | Query audit logs + provider logs for rows read; count distinct users | Record count and categories written down within 24h |
| 4. Classify | DPO + Compliance Officer set severity (S1/S2 here: BVN/NIN) | Rationale recorded |
| 5. Notify | Draft NDPC notification, sign-off, file by T+72h; prepare subject notice | Filing proof saved; subject email queued |
| 6. Recover | Force password resets, re-issue sessions, enhanced monitoring for 30 days | Owners + dates assigned |
| 7. Learn | Post-incident review within 10 business days | Actions tracked to closure in this file |

**Failure modes this drill is designed to catch:** no one owns T+0, logs
rotated before scoping, severity decided too late, notification drafted from
memory instead of evidence, subjects told before the NDPC (or vice versa out
of order without reason).

## 8. Related documents

- [DPO_ASSESSMENT.md](DPO_ASSESSMENT.md) — who runs this runbook (DPO/DCMI).
- [DATA_RETENTION_SCHEDULE.md](DATA_RETENTION_SCHEDULE.md) — what must survive
  an erasure, and for how long.
- [STR_PROCEDURE.md](STR_PROCEDURE.md) — suspicious-transaction reporting
  (NFIU), a *separate* 24-hour clock; a fraud breach may trigger both.
- Admin surfaces: **Compliance** tab (audit CSV export), **Risk** tab
  (alerts/STR), **Privacy** tab (exports, erasure, suppression).