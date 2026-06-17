---
description: Perform a new round of rigorous code review and save the report to ./ref-only/code-review-{next-idx}.md
---

You are an expert Go backend engineer and security auditor.

We have already completed one or more rounds of code reviews on this **court booking backend (場地預約系統後端)** and implemented improvements based on the findings. The reports from previous rounds are saved at ./ref-only/code-review-{idx}.md, and the deliberate design decisions that we have explicitly chosen NOT to fix are consolidated at ./ref-only/code-review-decisions.md (this file may not exist yet if no decision has been recorded).

**To save context, you do NOT need to read every previous report.** For the next round, read only:

1. The latest report: ./ref-only/code-review-{latest-idx}.md (the highest-numbered file), for the most recent state and outstanding issues.
2. ./ref-only/code-review-decisions.md (if it exists), for the list of known issues that are intentional and will NOT be fixed — do not re-report these.

Only if you need to confirm a specific detail or trace the history of a particular finding should you open the older ./ref-only/code-review-{idx}.md reports.

> Project stack, architecture, module map, and coding conventions are in the root `CLAUDE.md` (and `README.md` §開發規範) — assume them as background.

Please use those for context, and perform another rigorous round of code review on the current court booking backend. Actively look for logical flaws, edge cases, security risks, or scalability issues. Prioritize maintainability, readability, and defensive programming over negligible performance micro-optimizations.

### Specific Areas of Focus

- **Verification & Regression:** Based on the latest report (./ref-only/code-review-{latest-idx}.md), verify whether the previous issues were properly and safely addressed. Ensure the recent improvements did not introduce new bugs or regressions. Cross-check ./ref-only/code-review-decisions.md so you don't re-report deliberately accepted decisions.
- **Concurrency & State:** Look for potential race conditions (TOCTOU) under high concurrency — especially booking creation/update (double-booking the same resource & time slot), availability queries, and pickup enrollment quota/capacity control. Check whether reads-then-writes are protected by transactions, row locks (`SELECT ... FOR UPDATE`), or database-level exclusion/unique constraints.
- **Data Consistency & Error Handling:** Ensure the system fails safely without leaving booking state, file references, or enrollment counts inconsistent (e.g. orphaned files / dangling references on partial failure). Confirm domain errors map to correct HTTP status codes (e.g. foreign-key conflicts as 409, not 500), and that authorization (role/permission, soft-deleted/inactive users) is enforced on every path.

### Execution & Delivery

1. Generate the comprehensive review report in **Traditional Chinese** (繁體中文, using Taiwan terminology, e.g., 執行緒, 記憶體, 伺服器, 模組, 交易, 競態).
2. Format the report using clear Markdown headings, code blocks, and bullet points. Reference code locations as clickable links (e.g. `[internal/booking/service.go:96](internal/booking/service.go#L96)`), consistent with the existing reports.
3. **Crucial:** Save the entire output directly to ./ref-only/code-review-{next-idx}.md. Ensure the file is created or overwritten with the complete content.
