# PropVest Licence Posture Memo

**Status:** Draft for counsel review — owner: Founders + Compliance Officer
**Effective:** Phase 9 (Oct 2026)
**Purpose:** record which licences the product plausibly needs *before* go-live,
and who owns each decision. Nothing here is legal advice; every item marked
**[COUNSEL]** must be confirmed by a Nigerian lawyer before launch.

---

## 1. Payment rails — operate under the processor's licence

**Decision:** PropVest does **not** seek its own CBN licence at launch. All
payment acceptance and payout runs through **Paystack** (a CBN-licensed
Switching & Processing company on the CBN PSP register¹). Settlement arrives
into the platform's bank account on Paystack's schedule; the app models the
batches (Phase 7 settlement import) but never holds customer e-money of its
own.

**What this means in practice**

- No card PANs touch our infrastructure — Paystack's hosted flows own that.
- Webhooks are signature-verified, replay-protected and reconciled against
  our ledger (Phase 2/7 verification).
- The health endpoint reports Paystack reachability; a second PSP behind the
  same `payments` interface is the redundancy plan if volume grows.

**The sensitive point [COUNSEL]:** the wallet. The app keeps a user-facing
`wallets` balance (main / locked / earnings). If PropVest — rather than a
licensed institution — is the party that *owes* the customer that balance,
the arrangement may constitute issuing e-money and require an MMO (or other
CBN) licence. The flow of funds must be reviewed end to end:

1. Who holds the money at rest? (PropVest's account vs. Paystack's
   settlement/collection account vs. a dedicated trust account.)
2. Who owes the customer, and under what instrument?
3. Who controls settlement and payout?

**Mitigation until counsel rules:** keep balances *synthetic* — customer
money enters and leaves only through Paystack; deposits are credited on
provider confirmation; withdrawals are paid out via Paystack transfers. Do
not market the wallet as "stored value". Tiered KYC transaction limits under
the CBN mobile-money framework are **not** quoted here because the figures
could not be verified from a primary source — obtain the operative guideline
before publishing any limits.¹

## 2. Investment product — SEC perimeter is the real question

**Decision [COUNSEL]: treat SEC registration as *likely required*, not
optional.** PropVest pools investors' money into income-producing real
estate and distributes returns (performance fee on distributed profit). That
is the shape of a **collective investment scheme**, and real-estate pooling
sits in the **REIS/REIT** family under the **Investment and Securities Act
2025**, which expanded the CIS forms the SEC can regulate.² The product's
label ("property shares", "off-plan units") does not settle classification —
substance does.

Typical structures to discuss with counsel:

| Structure | What PropVest becomes | Registration burden |
|---|---|---|
| Unit-trust CIS | Manager/sponsor, with an independent **trustee** holding the assets | Scheme + operator registration with SEC |
| REIS / REIT wrapper | Sponsor/manager of a registered scheme | Scheme registration + SEC rules on custody and valuation |
| Direct ownership (investors hold titles) | Platform is an agent/marketing surface | Lower SEC burden, but land-title and consumer-protection issues move to the foreground |

Acting as a "sponsor" or "distributor" is not automatically a regulated role,
but soliciting investors or placing securities may be — **have counsel map
each party's actual activity** to the current SEC rules rather than relying
on titles.²

**Consequence for sequencing:** Phase 7 shipped the fee engine (1% capped
₦2,000 on withdrawal; 10–15% performance fee on distributed profit). If SEC
registration is required, fees and marketing of the product cannot go live
to the public ahead of it — this memo therefore **gates the same go-live
checkbox** as the compliance pack.

## 3. AML/CFT — NFIU reporting, not a licence

Already handled in Phase 8: goAML registration (see
[GOAML_REGISTRATION.md](GOAML_REGISTRATION.md)), STR/CTR procedures
([STR_PROCEDURE.md](STR_PROCEDURE.md)), and transaction monitoring with an
alert queue. Confirm PropVest's status as a **reporting entity** under the
MLPPA 2022 — [COUNSEL] — and start goAML onboarding early; it has lead time.

## 4. Data protection — NDPA 2023 obligations apply immediately

No licence, but enforceable duties: implementable data-subject rights,
consent evidence, 72-hour breach notification ([BREACH_RUNBOOK.md](BREACH_RUNBOOK.md)),
and a **DPO if we qualify as a data controller of major importance**
([DPO_ASSESSMENT.md](DPO_ASSESSMENT.md)). Phases 6 and 9 shipped the code;
the assessment and pack are this deliverable.

## 5. Decision log

| # | Question | Owner | Status |
|---|---|---|---|
| L1 | Does the wallet need an MMO licence, or is a processor/trust arrangement sufficient? | Founders + CBN counsel | **Open — blocks go-live** |
| L2 | Is the pooled real-estate product a CIS/REIS requiring SEC registration? | Founders + SEC counsel | **Open — blocks fee launch** |
| L3 | PropVest = NFIU reporting entity? goAML onboarding | Compliance Officer | Registration doc drafted (Phase 8) |
| L4 | DPO/DCMI classification under GAID | DPO-designate | Assessment drafted ([DPO_ASSESSMENT.md](DPO_ASSESSMENT.md)) |
| L5 | NDIC disclosure wording for customer communications | Compliance + counsel | Drafted ([NDIC_DISCLOSURE.md](NDIC_DISCLOSURE.md)) |

## Sources

1. CBN Payment Service Providers register (Paystack listed under Switching &
   Processing): https://www.cbn.gov.ng/PaymentsSystem/PSPs.html
2. Investment and Securities Act 2025 (SEC-hosted PDF):
   https://home.sec.gov.ng/documents/1319/Investments_and_Securities_Act_2025_x9rSXtI.pdf
3. CBN 2021 Mobile Money Services framework — figures **not** quoted here;
   obtain the operative guideline before publishing limits.