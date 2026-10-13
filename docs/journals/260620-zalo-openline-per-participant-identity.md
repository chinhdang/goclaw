# Zalo Openline — per-participant identity (Phase 1)

**Date:** 2026-06-20 · **Branch:** `bitrix24-channel-core-rebased` · **Commit:** `c1290eaa` · **Task:** B24:2794

## Problem

Every Zalo customer in a shared Bitrix24 Open Channel group collapsed onto one
identity. Two-layer root cause (empirically verified from live logs):
1. `handle.go` set `senderID = evt.Params.FromUserID = "960"` — the connector
   proxy, shared by every customer.
2. `gateway_consumer_normal.go` then forced `userID = "group:{channel}:{chatID}"`
   → one shared USER.md / memory for the whole group.

Live logs also showed an **operator** (`IS_CONNECTOR=N`) typing text that begins
`[Trung Hee] …` — a real forged-tag vector.

## Approach (Path A, zero migration)

The Synity connector prepends a 3-token sender tag `[Name] #uid #msgId`. goclaw
now parses it:
- **3-token** → mint synthetic per-person senderID `openlines:{instance}:{chat}:{uid}`,
  echo `#msgId` only on the reply.
- **legacy 1-number** `[Name] #msgId` and **name-only** `[Name]` → unchanged.

Identity minting is gated on `FromIsConnector == true`, so an operator forged tag
never impersonates a customer. Untagged / parse-fail messages degrade safely to
the group-level userID.

`deriveGroupUserID()` (extracted pure helper) sets the scope precedence:
Discord guild → openline participant → `group:{channel}:{chatID}` fallback.

## Two things the plan got wrong (caught before/at review)

1. **`MetaKeyMessageID` override would break the v2 reply-link.** The plan said to
   overwrite it with the connector `msgId`. But that key feeds `fields.replyId`,
   which expects a Bitrix MESSAGE_ID (6-digit) — the connector msgId is a 13-digit
   Zalo id. Overriding would have set an invalid replyId for *all* openline replies
   (3-token and legacy). Kept `MetaKeyMessageID = evt.Params.MessageID`; the
   connector msgId lives only in the echo prefix. Confirmed against `send_v2.go`
   + the empirical report before writing code.
2. **Synthetic senderID → invalid `[USER=…]` BBCode (code-review catch).** The new
   `openlines:…` senderID flowed into `bitrix_address_user_id` and would have
   emitted `[USER=openlines:…][/USER]` in replies. Fixed at `buildAddressMention`
   with an `isNumericID` guard — preserves legacy `[USER=960]`, suppresses
   synthetic ids. General defense, not just openline-specific.

## Verification

`go build ./...` (PG) + `go build -tags sqliteonly ./...` (Desktop) + `go vet ./...`
clean. New table/handle/consumer/send tests + existing OL/send/consumer suites
green. (Toolchain ran in a `golang:1.26` Docker container — Go isn't on host PATH.)

## Surface parity

Gateway/adapter = the work. API contract: `ChannelContact` unchanged, per-person
rows appear via existing `ListContacts`. Web UI: contacts + merge UI already
render per-person rows — no change. CLI: N/A. Docs: none warranted.

## Carry-over for Phase 2 (blocked)

Phase 2 (cross-group rollup) is gated on a prod empirical check: after the
connector ships `#uid`, verify the same person yields the same uid across two
groups. Also: `extractUIDFromSenderID` will split the synthetic id on `:`, but
the channel instance name (`c.Name()`) isn't validated colon-free — use a
colon-tolerant parse (last segment = uid) or validate the name.
