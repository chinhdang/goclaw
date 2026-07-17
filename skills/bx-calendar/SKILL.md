---
name: bx:calendar
description: "Synity Bitrix24 Calendar via MCP Synity. Use for: creating meetings, scheduling events, follow-up reminders, checking team availability, syncing CRM activities to calendar. NOT for CRM records (use bx:crm) or project tasks (use bx:task)."
argument-hint: "<subcommand> [args...]"
version: "2.0.0"
---

# /bx:calendar — Bitrix24 Calendar Operations (Synity)

**Tool**: MCP Synity at `b24-mcp.synity.so`. Calendar REST methods are called through the MCP with
`codemode.request({ method, path, body })` — the raw REST passthrough used across the `bx:*` suite.
No webhook and no local `curl`/`jq`.

> ⛔ `bx:crm` = CRM entities (contact/deal). NOT for calendar events.  
> ⛔ `bx:task` = project task management. NOT for calendar events.  
> ✅ All calendar events, meetings, reminders, scheduling → this skill.

---

## Pre-flight

- MCP Synity (`b24-mcp.synity.so`) must be connected in the session. If it is not, tell the user to
  authorize it (claude.ai connector settings, or `/mcp` in an interactive session) — this skill has
  no fallback transport.
- No environment variable or local tool (`jq`/`curl`) is required.

---

## Detect Intent → Load File

| User intent | Load file |
|-------------|-----------|
| Create meeting, schedule event, link to CRM contact/deal | `meeting.md` |
| Set reminder, follow-up after meeting | `reminder.md` |
| Check availability, find free time slot | `availability.md` |
| Sync CRM activity to calendar event | `sync.md` |

---

## Mandatory Workflow

```
1. Detect intent → load the correct subfile above
2. Follow SOP in that file (timezone, CRM binding)
3. Execute the REST method via `codemode.request({ method, path, body })`
4. Verify result
```

---

## REST Method Basics

Calendar methods are invoked **through MCP Synity** with `codemode.request(...)`, not `curl`. The
`path` is `/<method>/` and `body` carries the params (flat — calendar methods do NOT wrap params in
`fields`, unlike CRM):

```js
await codemode.request({
  method: 'POST',
  path: '/calendar.event.add/',
  body: { name: '...', type: 'user', ownerId: 1, from: '...', to: '...' }
})
```

**Input parameters**: camelCase (`name`, `ownerId`, `from`, `to`)
**Response fields**: UPPER_SNAKE_CASE (`TZ_FROM`, `TZ_TO`, `ID`)

| Operation | REST method |
|-----------|-------------|
| Create event | `calendar.event.add` |
| Update event | `calendar.event.update` |
| Get / list events | `calendar.event.get`, `calendar.event.getlist` |
| Delete event | `calendar.event.delete` |
| List sections | `calendar.section.getlist` |

> **CRM binding**: write with `crm_fields[]` (`C_`/`CO_`/`L_`/`D_`). `UF_CRM_CAL_EVENT` is read-only
> output — see `meeting.md` for the full binding rules.
