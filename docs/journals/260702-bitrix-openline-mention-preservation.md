# Bitrix24 Openline — bracket mention preservation in agent replies

**Date:** 2026-07-02 · **Branch:** `bitrix24-channel-core-rebased` · **Commit:** `9aee9e5f` · **Plan:** `plans/260702-1547-bitrix-openline-mention-preserve/`

## Problem

Bitrix24 Open Channel relays inbound customer messages with `[Full Name]` bracket mentions. A downstream connector resolves those mentions via exact string match — case matters, brackets matter. Agent replies sometimes echoed names bare (no brackets) or with different casing, silently breaking resolution. Not a cosmetic bug: unresolved mentions mean the connector can't route the reply to the right participant.

## Approach

A deterministic code layer in `internal/channels/bitrix24/openline_mentions.go` that:
1. **Extracts** all `[Name]` mentions from inbound Open Channel text into a per-chat roster.
2. **Caches** that roster in memory (TTL 24 h, size-capped at 500 entries per chat, concurrency-safe via `sync.RWMutex`, injectable clock interface for TTL tests).
3. **Re-wraps** matching name tokens in outbound agent replies with canonical (roster) casing before the Markdown→BBCode conversion runs.

The entire feature is gated on Open Channel presence. DM and group chats are untouched — verified via negative tests that assert zero mutation on non-openline messages.

## Interesting engineering decisions

### RE2 constraint workaround

Go's `regexp` package is RE2-safe: no lookaround (`(?<=...)` / `(?=...)`). The naive approach — match `[Name]` but skip already-bracketed occurrences in the output — requires lookbehind. Worked around with manual rune inspection: after a regex match locates a candidate token in the outbound string, the code checks the rune immediately before and after the match position. If they are already `[` / `]`, skip. Word-boundary logic likewise uses `unicode.IsLetter`/`IsDigit` rune checks instead of `\b`. Slightly verbose but transparent and testable.

### Single-token vs multi-token name matching

Multi-token roster names (e.g. `Nguyễn Văn An`) match case-insensitively — the tokens are distinctive enough. Single-token names are matched exact-only. The temptation was to use token-length as a discriminator (e.g. "≥5 runes → case-insensitive"), but Vietnamese single names like `Nam`/`Bình`/`Hà` are common vocabulary words at any length. Colliding a mention of a customer named `Nam` with the word "nam" (south) in the same sentence would produce garbage. Exact-only for single tokens is the safe floor.

### Roster cache design

`openlineRosterCache` is a value struct per `OpenLineHandler`, not a package-level global, so multi-tenant deployments with multiple handler instances get isolated caches. The injectable `clockFn func() time.Time` field lets unit tests fast-forward TTL expiry without `time.Sleep`. Size cap evicts the oldest entry (by `lastSeen` timestamp) when the chat's roster hits 500 — enough for any realistic Open Channel deployment.

### go fix scope-creep catch

CLAUDE.md mandates `go fix ./...` before committing. Running it silently modernized ~35 files across `internal/skills/`, `internal/tools/`, `internal/http/` — packages with nothing to do with this plan. Caught via `git status --short` before staging. Reverted everything outside the plan's actual touch-set with targeted `git restore` calls. The feature commit contains exactly the 11 files the plan specified.

**Lesson for future sessions:** `go fix ./...` is repo-wide. Always audit `git status --short` immediately after running it and restore unrelated files before staging. Do not let a chore command contaminate a feature commit.

## Verification

All 4 plan phases implemented via strict TDD — tests written first, compile-fail confirmed red, then implemented to green — in one sitting. Two parallel background subagents ran independent verification:

- **tester:** full `go build ./...` + `go build -tags sqliteonly ./...` + `go vet ./...` + `go test -race ./...` matrix, plus 9 self-designed adversarial edge cases (regex metacharacters in names, mixed-case BBCode collisions, empty roster entries, 3-level substring nesting). All passed.
- **code-reviewer:** verdict APPROVE WITH MINOR FIXES, two findings both fixed and re-verified green:
  1. The mention-extraction regex allowed `|` inside bracket matches. Since the roster is pipe-joined in the `MetaKeyOpenlineRoster` metadata value, a name literally containing a pipe would have corrupted the split. Excluded `|` from the character class.
  2. `enforceOpenlineMentionBrackets` was missing the NUL-strip guard that `markdownToBitrixBBCode` already applies to its own NUL-framed placeholder scheme. Without it, a name that happened to collide with a NUL placeholder could have corrupted the BBCode output. Added the guard.

**Environment friction:** Go was not installed in the sandbox at session start. Installed via `brew install go` (landed 1.26.4) to enable compile/test verification as mandated by the post-implementation checklist. Worth noting: this environment doesn't have Go pre-provisioned — budget ~2 min for install at the start of any Go session here.

## Deliberate incomplete item — persona layer deferred

The plan included a supplementary step: add a Vietnamese SOUL.md instruction block for agent "mai" to echo `[Name]` mentions exactly, reinforcing the bracket convention at the LLM level. Investigated two automated paths and found both blocked:

- The HTTP REST API has no agent-level context-file endpoint (only per-user USER.md via `PUT /v1/agents/{id}/instances/{userID}/files/{fileName}`).
- The WS RPC method that does exist (`agents.files.set`, used by the Dashboard) is deliberately outside the `goclaw-admin` skill's administration allowlist.

Rather than bypass that guardrail, raised the question with the user. Decision: skip for now. The deterministic code layer is the guaranteed floor — agent replies are bracket-corrected regardless of whether the persona nudge ever lands. A background task chip was left so the user can apply the persona block later via Dashboard UI or a confirmed SQL UPDATE.

## Surface parity

Gateway/channel layer = the work. API contract unchanged. Web UI: contacts and chat already render openline replies — no change. CLI: N/A. Docs: none warranted.
