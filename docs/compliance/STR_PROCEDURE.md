# PropVest STR & CTR Procedure

**Status:** Signed-off procedure — owner: Compliance Officer
**Effective:** Phase 8 (Oct 2026)
**Clock:** STR = **24 hours** from determination (MLPPA 2022). CTR = **7 days**
from the transaction day.

---

## 1. What triggers an STR

Any suspicion — regardless of amount. There is **no minimum value threshold**
for an STR. The platform's own triggers:

1. A **screening hit** (sanctions/PEP match) — the system freezes the account,
   escalates the alert and drafts the STR automatically.
2. An analyst **escalates** any risk alert (`Escalate → STR` in the admin Risk
   panel) — the 24-hour countdown starts at that moment.
3. Any employee's intuition — report to the Compliance Officer; do not tip off
   the customer.

## 2. Filing steps (STR)

1. **Detect** — alert appears in the Risk panel queue (or is raised by staff).
2. **Triage** — move `open → in_review`. Gather evidence: transaction IDs,
   timestamps, amounts, device/IP data from the audit log.
3. **Decide** — if suspicion is confirmed, `Escalate`. This drafts the STR
   package with a deadline of **escalation + 24h** and shows the countdown on
   the STR screen.
4. **Narrate** — complete the narrative: who, what, when, where, why
   suspicious, amounts, and what the platform did (freeze/flag/block). The
   draft already names the rule and the alert.
5. **File** — log in to the NFIU **goAML portal**
   (see [GOAML_REGISTRATION.md](GOAML_REGISTRATION.md)), submit the report,
   copy the portal's reference number.
6. **Record** — back in the Risk panel, *Mark filed* and paste the goAML
   reference. Filing stamps `filed_at`, writes an audit row (`str.filed`) and
   clears the red overdue state.

**Overdue = red on the dashboard.** An STR past its 24h deadline is a
regulatory breach: escalate to the CEO the same day and file immediately.

## 3. What NOT to do

- **Never tip off the customer.** No hints, no "your account is under review
  for compliance reasons" beyond scripted wording.
- **Never delete or edit a filed STR.** Corrections go to NFIU via the portal
  as amendments.
- **Do not block the filing because the customer is a VIP.** Determination is
  the clock's start; the report is filed either way.

## 4. CTR (cash threshold)

- ₦**5,000,000** individual / ₦**10,000,000** body corporate, **cash**
  transactions → CTR within 7 days.
- The platform's flows are electronic, so the daily scan flags days where a
  user's deposit volume crossed the threshold and labels the row
  *"confirm whether a cash CTR applies"*. The Compliance Officer makes that
  call — an electronic transfer is not a cash transaction, but the volume is
  reported for review.
- File via goAML (CTR report type), then *Mark filed* on the CTR row with
  notes. Notes are mandatory and audited.

## 5. Escalation matrix

| Situation | Action |
|---|---|
| STR deadline < 6h remaining | Notify Compliance Officer (dashboard turns amber) |
| STR overdue | Notify CEO + file immediately; log the delay |
| Screening hit on an active account | Account frozen automatically — review within 24h |
| Suspected internal complicity | Report directly to the CEO; do not use internal channels |

## 6. Retention

STR/CTR packages, evidence and the audit trail: **minimum 5 years** after the
customer relationship ends. Backups cover these tables (see Phase 3 backup
policy); the monthly restore drill proves they are recoverable.