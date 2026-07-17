# bx:calendar — Reminder (Follow-up + Pre-meeting)

> Execute every method below through MCP Synity via `codemode.request({ method, path, body })`. No `curl`.

---

## Two Reminder Use Cases

### Use Case 1 — Pre-meeting notification

Add a `remind` array when creating the event via `calendar.event.add`:

```js
await codemode.request({ method: 'POST', path: '/calendar.event.add/', body: {
  name: 'Demo call',
  type: 'user',
  ownerId: 1,
  from: '2026-05-16T10:00:00+07:00',
  to: '2026-05-16T11:00:00+07:00',
  section: 3,
  timezone_from: 'Asia/Ho_Chi_Minh',
  timezone_to: 'Asia/Ho_Chi_Minh',
  remind: [{ type: 'min', count: 15 }]
} })
```

**`remind` type values**: `"min"` | `"hour"` | `"day"`

**Standard offsets:**
- Pre-meeting: `{type: "min", count: 15}` (15 min before)
- Long event: `{type: "hour", count: 1}` (1 hour before)

---

### Use Case 2 — Post-meeting follow-up

Create a **new event** scheduled `+1 business day` after meeting end.

**Business day calculation rule:**
- If meeting ends Mon–Thu → follow-up = next day at 09:00
- If meeting ends Fri → follow-up = Monday at 09:00
- If meeting ends Sat/Sun → follow-up = Monday at 09:00
- VN public holidays: out of scope (skip manually if needed)

Call `calendar.event.add` with the calculated datetime (`from`/`to` = +1 biz day at 09:00),
binding CRM via `crm_fields[]` (not `UF_CRM_CAL_EVENT`, which is read-only output):

```js
await codemode.request({ method: 'POST', path: '/calendar.event.add/', body: {
  name: 'Follow-up: Demo call with Nguyen Van A',
  type: 'user',
  ownerId: 1,
  from: '2026-05-17T09:00:00+07:00',
  to: '2026-05-17T09:30:00+07:00',
  section: 3,
  timezone_from: 'Asia/Ho_Chi_Minh',
  timezone_to: 'Asia/Ho_Chi_Minh',
  crm_fields: ['D_42'],
  remind: [{ type: 'min', count: 30 }]
} })
```

---

## Verify Checklist

- [ ] Event `ID` returned
- [ ] For pre-meeting: `remind` array present in event details
- [ ] For follow-up: datetime is a business day at 09:00 VN time
- [ ] CRM deal/contact linked (`UF_CRM_CAL_EVENT` populated in the read-back)
