# bx:calendar — Availability (Find Free Slots)

> Execute every method below through MCP Synity via `codemode.request({ method, path, body })`. No `curl`.

---

## Query Events for a User

Call `calendar.event.getlist` to get all events for a user in a date range:

```js
await codemode.request({ method: 'POST', path: '/calendar.event.getlist/', body: {
  type: 'user', ownerId: 1, from: '2026-05-16', to: '2026-05-16'
} })
```

Keep from each result: `ID`, `NAME`, `DATE_FROM`, `DATE_TO`.

**Parameters:**
| Parameter | Value |
|-----------|-------|
| `type` | `"user"` for personal calendar |
| `ownerId` | Bitrix24 user ID |
| `from` | Date string `"YYYY-MM-DD"` |
| `to` | Date string `"YYYY-MM-DD"` |

---

## Free Slot Algorithm

1. Fetch busy events for each attendee in the target date range
2. Normalize all event times to VN timezone (`Asia/Ho_Chi_Minh`, UTC+7)
3. Merge busy intervals per attendee → find gaps ≥ `meetingDuration`
4. Intersect free gaps across all attendees
5. Filter to working hours: **08:00–17:30 VN** (configurable)

**Output format:**
```
Free slots for [date]:
- 08:00–10:00 (2h)
- 13:00–15:30 (2.5h)
- 16:00–17:30 (1.5h)
```

---

## Multi-User Availability Check

Call `calendar.event.getlist` once per attendee (users `1`, `2`, `3`), each with
`body: { type: 'user', ownerId: <uid>, from: '2026-05-16', to: '2026-05-16' }`, then apply the
Free Slot Algorithm across the combined busy intervals.

---

## Notes

- Working hours assumption: 08:00–17:30 VN — adjust if user has different hours
- `calendar.event.getlist` returns events where the querying user has read access
- Response `DATE_FROM` / `DATE_TO` are in UPPER_SNAKE_CASE
- All-day events: `DATE_FROM` has no time component — treat as blocking 00:00–23:59
