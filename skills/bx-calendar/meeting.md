# bx:calendar — Tạo Lịch Họp (Meeting SOP)

> Skill này là **SOP** (quy trình thực hiện), không phải API doc.
> Không chứa curl snippet. Dùng Bitrix Synity MCP + Google Calendar MCP để thực hiện.

---

## Detect Intent → Chọn Flow

```
Bước 0: Kiểm tra session có tool nào tạo được Google Calendar event với conferenceData không?
         (tìm tool có capability: create_event + conferenceData/videoConference)
         - Có → Flow A (Google Calendar + Bitrix)
         - Không → Flow B (Bitrix only)
```

> KHÔNG hardcode tên MCP (ggws, gcal, google-workspace...). Detect bằng capability.

---

## Flow A — Google Calendar MCP + Bitrix (Đầy đủ: Meet link + CRM binding + invite)

### Bước A0: Thu thập thông tin

Hỏi user (nếu chưa có):
- Tiêu đề cuộc họp
- Ngày giờ bắt đầu / kết thúc (timezone +07:00)
- Địa điểm (nếu có)
- CRM entity cần binding: contact ID, deal ID, company ID
- Email khách bên ngoài cần invite (nếu có)
- **Có muốn mời Bitrix employee (user nội bộ) nào không?**
  - Nếu có → hỏi tên/email để tìm user ID qua `im.user.list` hoặc `user.get`

### Bước A1: Tạo Google Calendar event

Dùng **Google Calendar MCP**:
- `summary`: tiêu đề cuộc họp
- `startTime`: UTC datetime — **tính từ local +07:00, trừ 7 giờ** → `2026-07-03T08:00:00Z` (= 15:00+07:00)
- `endTime`: tương tự — ví dụ `2026-07-03T10:00:00Z` (= 17:00+07:00)
- `location`: địa điểm plain text
- `description`: nội dung cuộc họp
- `attendees`: danh sách email khách bên ngoài
- `createConferenceLink`: true (tạo Google Meet)
- **KHÔNG truyền `timeZone` parameter** — timezone bị override làm sai giờ

Nhận lại: `eventId` + `conferenceUrl` (Meet link)

### Bước A2: Chờ CalDAV sync

Dùng `wait(90)` → Bitrix tự đồng bộ từ Google Calendar qua CalDAV.

### Bước A3: Poll Bitrix — tìm event vừa sync

Dùng **Bitrix MCP**: `calendar.event.get` với `type=user`, `ownerId=<userId>`

Fetch tất cả events → match client-side: `DAV_XML_ID == eventId + "@google.com"`

Nếu không tìm thấy:
- `wait(60)` → retry lần 2
- `wait(60)` → retry lần 3
- Nếu vẫn không có → báo user kiểm tra cấu hình CalDAV sync trong Bitrix portal

### Bước A4: Update Bitrix event

Dùng **Bitrix MCP**: `calendar.event.update` với event ID vừa tìm được:

**CRM binding** (param `crm_fields[]`, KHÔNG dùng `UF_CRM_CAL_EVENT`):
| Entity | Prefix |
|--------|--------|
| Contact | `C_<id>` |
| Company | `CO_<id>` |
| Lead | `L_<id>` |
| Deal | `D_<id>` |

**Ví dụ**: contact #1668 → `crm_fields[]=["C_1668"]`

**DESCRIPTION**: append Meet link + Maps URL:
```
🎥 Google Meet: <conferenceUrl>
📍 Địa điểm: <location>
🗺️ Maps: https://maps.google.com/?q=<url-encoded location>
```

**Invite Bitrix employees** (nếu có): thêm vào `ATTENDEES_CODES[]` danh sách user ID nội bộ

### Bước A5: Xử lý duplicate (nếu có)

Google account có nhiều calendar sync → có thể tạo ra 2+ Bitrix events cùng `DAV_XML_ID`:
- Fetch tất cả events match `DAV_XML_ID`
- Giữ lại event có CRM binding (`UF_CRM_CAL_EVENT` không rỗng)
- Xóa các event duplicate còn lại bằng `calendar.event.delete`

### Bước A6: Verify

- Bitrix event: ID, tên, giờ đúng, `UF_CRM_CAL_EVENT` có entity, DESCRIPTION có Meet link
- Google Calendar event: giờ đúng (+07:00), attendees đủ, Meet link tồn tại

---

## Flow B — Bitrix only (Không có Google Calendar MCP)

### Bước B0: Thu thập thông tin

Hỏi user (nếu chưa có):
- Tiêu đề, ngày giờ, địa điểm
- CRM entity cần binding
- **Có muốn mời Bitrix employee nào không?** → lấy user ID

### Bước B1: Tìm section ID

Dùng **Bitrix MCP**: `calendar.section.getlist` với `type=user`, `ownerId=<userId>` → lấy section ID mặc định

### Bước B2: Tạo Bitrix event trực tiếp

Dùng **Bitrix MCP**: `calendar.event.add`:
- `name`: tiêu đề
- `type`: `user`
- `ownerId`: user ID
- `from`: `"2026-07-03T15:00:00+07:00"` (ISO 8601 với offset)
- `to`: `"2026-07-03T17:00:00+07:00"`
- `section`: section ID từ bước B1
- `timezone_from`: `"Asia/Ho_Chi_Minh"`
- `timezone_to`: `"Asia/Ho_Chi_Minh"`
- `crm_fields[]`: `["C_1668", "D_42"]` (KHÔNG dùng `UF_CRM_CAL_EVENT`)
- `ATTENDEES_CODES[]`: user ID nội bộ cần invite (nếu có)

### Bước B3: Verify

- Event ID trả về (không null)
- `TZ_FROM` = `"Asia/Ho_Chi_Minh"`
- `UF_CRM_CAL_EVENT` có entity ID

---

## CRM Prefix Reference (đã verify)

| Entity | Prefix | Ví dụ |
|--------|--------|-------|
| Contact | `C_<id>` | `C_1668` |
| Company | `CO_<id>` | `CO_7` |
| Lead | `L_<id>` | `L_33` |
| Deal | `D_<id>` | `D_42` |

> **Write param**: `crm_fields[]` (top-level, form-encoded)
> **Read-only output**: `UF_CRM_CAL_EVENT` — ghi vào đây bị silently ignored

---

## Timezone Rules (CRITICAL)

**Flow A (Google Calendar MCP):**
- Tính UTC từ local: `15:00+07:00` → trừ 7h → `08:00 UTC` → truyền `"2026-07-03T08:00:00Z"`
- **KHÔNG truyền `timeZone` parameter** — nó override UTC offset, gây sai giờ

**Flow B (Bitrix direct):**
- Truyền ISO 8601 với offset: `"2026-07-03T15:00:00+07:00"`
- Luôn set `timezone_from` + `timezone_to` = `"Asia/Ho_Chi_Minh"`

---

## Điều kiện hoạt động

**Flow A (đầy đủ):**
- Bitrix Synity MCP trong session
- Google Calendar MCP trong session (bất kỳ tên nào)
- Bitrix24 portal đã bật sync 2 chiều với Google Calendar
- GoClaw: tool `wait` phải được bật (không trong deny list)

**Flow B (cơ bản):**
- Chỉ cần Bitrix Synity MCP

---

## Lỗi thường gặp

| Lỗi | Nguyên nhân | Fix |
|-----|-------------|-----|
| Google event sai giờ (+4h) | Truyền `timeZone` override | Bỏ `timeZone`, dùng UTC |
| CRM binding không xuất hiện | Dùng `UF_CRM_CAL_EVENT` để ghi | Đổi sang `crm_fields[]` |
| 2 Bitrix events cùng nội dung | CalDAV sync 2 calendar cùng account | Xóa duplicate (bước A5) |
| Meet link không sync sang Bitrix | CalDAV không carry conferenceData | Patch DESCRIPTION thủ công (bước A4) |
| Không tìm thấy event sau wait(90) | CalDAV sync chậm hoặc chưa cấu hình | Retry 2 lần, kiểm tra Bitrix portal |
