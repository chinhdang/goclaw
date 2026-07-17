# bx:calendar — Sync (CRM Activity → Calendar Event)

> Execute every method below through MCP Synity via `codemode.request({ method, path, body })`. No `curl`.

---

## Direction

**One-way only: CRM Activity → Calendar Event.**

No native Bitrix24 webhook for 2-way sync. After creation:
- Editing the calendar event does NOT update the CRM activity
- Editing the CRM activity does NOT update the calendar event
- Manual update required if either changes after sync

---

## Field Mapping

| CRM Activity field | Calendar event parameter | Notes |
|-------------------|-------------------------|-------|
| `SUBJECT` | `name` | Event title |
| `START_TIME` | `from` | ISO 8601 + `+07:00` offset |
| `END_TIME` | `to` | ISO 8601 + `+07:00` offset |
| `DESCRIPTION` | `description` | Optional |
| `OWNER_TYPE_ID` + `OWNER_ID` | `crm_fields[]` | e.g. deal → `["D_42"]` |
| `RESPONSIBLE_ID` | `ownerId` | Bitrix24 user ID |

---

## Sync Steps

**Step 1 — Read CRM activity** — `codemode.request({ method: 'POST', path: '/crm.activity.get/', body: { id: 789 } })`.
Keep: `SUBJECT`, `START_TIME`, `END_TIME`, `OWNER_TYPE_ID`, `OWNER_ID`, `RESPONSIBLE_ID`.

**Step 2 — Map owner to CRM entity prefix**

| `OWNER_TYPE_ID` | Prefix |
|----------------|--------|
| 2 | `CO_` (contact) |
| 3 | `C_` (company) |
| 4 | `D_` (deal) |

**Step 3 — Create calendar event** — call `calendar.event.add`. Bind CRM through `crm_fields[]`
(not `UF_CRM_CAL_EVENT`, which is read-only output):

```js
await codemode.request({ method: 'POST', path: '/calendar.event.add/', body: {
  name: 'Meeting: <SUBJECT>',
  type: 'user',
  ownerId: '<RESPONSIBLE_ID>',
  from: '<START_TIME_ISO+07:00>',
  to: '<END_TIME_ISO+07:00>',
  section: 3,
  timezone_from: 'Asia/Ho_Chi_Minh',
  timezone_to: 'Asia/Ho_Chi_Minh',
  crm_fields: ['D_<OWNER_ID>']
} })
```

**Step 4 — Verify**
- [ ] Calendar event `ID` returned
- [ ] `UF_CRM_CAL_EVENT` (read-back) contains correct CRM entity
- [ ] `TZ_FROM` = `"Asia/Ho_Chi_Minh"` in response

---

## Limitation Note

> **After sync:** if you edit the calendar event or the CRM activity, the other side will NOT be updated automatically. This is a Bitrix24 platform limitation — no native 2-way sync via REST API. To update both, edit both manually.
