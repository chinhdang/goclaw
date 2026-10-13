# Google Calendar MCP (chính chủ) cho GoClaw — setup + chuỗi 4 lỗi

**Date**: 2026-07-19
**Severity**: Medium
**Component**: MCP infrastructure / OAuth / edge proxy
**Status**: Partially resolved — kết nối OK, tool call còn 403 chờ Developer Preview

## What Happened

Thay `taylorwilsdon/google_workspace_mcp` self-host (setup 01/07, xem `260701-google-workspace-mcp-goclaw-setup.md`) bằng **MCP server chính chủ của Google** — `https://calendarmcp.googleapis.com/mcp/v1`, ra mắt Cloud Next '26, public developer preview từ 01/05/2026. Server cũ `google-workspace` đã xoá chủ ý; chấp nhận mất Gmail/Drive/Docs (45 tools) để đổi lấy 8 tool calendar không phải nuôi hạ tầng.

Đăng ký server vào GoClaw dev, gỡ 4 lỗi nối tiếp, kết nối thành công (8/8 tools), grant cho agent `mai`. Nhưng tool call đầu tiên trả `The caller does not have permission` — chưa xong hẳn.

## The Brutal Truth

Mất gần 1 tiếng cho 4 lỗi mà **không cái nào là lỗi của Google Calendar MCP**. Ba cái đầu là tự mình: key sai, DCR bật nhầm, transport sai. Cái thứ tư là bug thật trong GoClaw. Và sau khi gỡ hết thì lỗi cuối cùng lại là thứ không code nào sửa được: project chưa được duyệt Developer Preview.

Bài học đắt nhất: `/tools` trả về 8 tools **không chứng minh OAuth hoạt động**. `tools/list` là metadata, không chạm dữ liệu. Đã ăn mừng sớm một nhịp.

## Technical Details

### Cấu hình cuối (server `019f79c5-6a5d-7b1e-b979-d04b5bdb5b44`)

```
url           https://calendarmcp.googleapis.com/mcp/v1
transport     streamable-http
oauth.use_dcr false
grant_type    authorization_code
auth_endpoint  https://accounts.google.com/o/oauth2/v2/auth
token_endpoint https://oauth2.googleapis.com/token
scope         calendar.events + calendar.readonly
require_user_credentials false   (token dùng chung cho mọi user)
```

Redirect URI đăng ký bên Google Cloud Console: `https://goclaw-dev.synity.so/v1/mcp/oauth/callback`.

### Lỗi 1 — API key GoClaw 401

Keychain có **hai** item cùng tên: `goclaw_api_key` (key cũ đã revoke, trùng `$GOCLAW_API_KEY` trong env) và `“goclaw_api_key”` với **ngoặc kép cong U+201C/U+201D** trong tên service (key còn hạn). `security find-generic-password -s goclaw_api_key` không bao giờ tìm ra cái thứ hai.

Phát hiện bằng `security dump-keychain | grep -i goclaw` — hex `0xE2809C...E2809D` chính là cặp ngoặc cong.

### Lỗi 2 — `use_dcr: true` với Google

Google không hỗ trợ Dynamic Client Registration:

```
accounts.google.com/.well-known/openid-configuration → registration_endpoint: null
```

Bắt buộc client_id/secret đăng ký sẵn. Đúng vết xe đổ của journal 01/07 nhưng ngược chiều: lần trước phải **tắt** DCR cho workspace-mcp single-user, lần này cũng phải tắt — chỉ khác là lần này còn phải nhập endpoint thủ công.

### Lỗi 3 — `transport: sse`

Docs Google ghi transport HTTP. Probe `POST initialize` kiểu streamable-http tới endpoint → 200. Đổi sang `streamable-http`.

### Lỗi 4 — `redirect_uri_mismatch` (bug GoClaw)

Google trả `Error 400: redirect_uri_mismatch`. Decode base64 trong `authError` param:

```
redirect_uri: http://goclaw-dev.synity.so/v1/mcp/oauth/callback
              ^^^^ http
```

Không lách được bằng cách đăng ký thêm URI http — Google cấm `http` cho domain public.

**Root cause**: `callbackURL()` (`internal/http/mcp_oauth.go:102`, commit `0ae55991`) gate nhánh reverse-proxy bằng `X-Forwarded-Host`:

```go
if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {  // Cloudflare không gửi header này
    proto := r.Header.Get("X-Forwarded-Proto")
    ...
}
if host := r.Host; host != "" {
    scheme := "http"
    if r.TLS != nil { scheme = "https" }   // TLS terminate ở edge → nil → http
    ...
}
```

Cloudflare giữ nguyên `Host` header, chỉ gửi `X-Forwarded-Proto: https`. Nhánh 2 không kích hoạt → rơi xuống nhánh 3 → `http://`.

**Bằng chứng loại trừ tunnel**: `bitrix_portals.state.public_url` trên cùng instance = `https://goclaw-dev.synity.so`. Nó dùng hàm khác — `derivePublicURLFromRequest()` (`internal/gateway/public_url_snapshot.go:136`) đọc `X-Forwarded-Proto` **độc lập với** `X-Forwarded-Host`. Cùng mục đích, hai logic lệch nhau. Thêm: SSL/TLS mode của zone = `full`, không phải Flexible.

**Workaround đã áp dụng** — Cloudflare Transform Rule `d721a3df5797440a95c439b3a62f9be5`, phase `http_request_late_transform`:

```json
{"expression": "(http.host eq \"goclaw-dev.synity.so\")",
 "action": "rewrite",
 "action_parameters": {"headers": {"X-Forwarded-Host": {"operation": "set", "value": "goclaw-dev.synity.so"}}}}
```

**Fix thật**: PR tech-synity/goclaw#6 — nhánh `r.Host` đọc thêm `X-Forwarded-Proto`, helper `forwardedProto()` xử lý header dạng list của chained proxy. 3 test mới, 2 test fail trước fix.

### Lỗi 5 (chưa xong) — `The caller does not have permission`

Trace `019f7a31-875e-7f3f-b26d-640c290128d7`, span `mcp_google_calendar__list_events`, 332ms, `calendarId: null`.

Loại trừ được bằng doc chính thức:
- **Không thiếu scope** — `list_events` nhận 1 trong 4: `calendar`, `calendar.events`, `calendar.events.readonly`, `calendar.readonly`. Config có 2 trong số đó.
- **Không thiếu `calendarId`** — param optional, default primary.

Nguyên nhân theo thảo luận google-gemini/gemini-cli#26017 (trùng error string):
1. Project chưa được duyệt **Google Workspace Developer Preview Program**.
2. Phải authorize bằng tài khoản **Workspace**, không phải Gmail cá nhân.

## What We Tried

1. `claude mcp list` + probe endpoint cũ → phát hiện `google-ws.synity.so` đã chuyển sang OAuth-protected, local `:8000` chết, LaunchAgents không load
2. Đọc `/v1/mcp/servers` → thấy `google-calendar` đã tạo, `tools: []`, `grants: []`
3. Probe `.well-known` của Google → xác định không có DCR
4. Probe 7 đường callback để tìm route thật (`/v1/mcp/oauth/callback` trả script `postMessage`, các đường khác là SPA fallback)
5. Decode `authError` param → lộ `http://`
6. Kiểm tra `bitrix_portals.state.public_url` → loại trừ tunnel/edge
7. `git log --all -S"use_dcr"` → tìm ra commit `0ae55991` có code MCP OAuth (không có trên branch đang làm việc)
8. Tạo Transform Rule qua CF API → authorize thành công → 8 tools
9. Trace agent `mai` → lộ lỗi 403 của Google

## Lessons Learned

1. **`/tools` ra kết quả ≠ OAuth hoạt động.** `tools/list` là metadata. Phải gọi một tool thật (`list_calendars`) mới verify được. Journal 01/07 dạy "luôn test `/tools` sau đăng ký" — đúng nhưng chưa đủ.
2. **Đặt tên keychain item cẩn thận.** Ngoặc kép cong lọt vào tên service (do copy từ chat/doc) tạo item vô hình với mọi lệnh tìm bằng tên thẳng. Ba vòng debug chỉ vì hai ký tự.
3. **Hai hàm cùng mục đích trong một codebase là mầm bug.** `callbackURL()` và `derivePublicURLFromRequest()` cùng dựng public URL từ request nhưng lệch logic. Bug chỉ lộ khi chạy sau proxy giữ Host header.
4. **Đừng sửa hạ tầng khi chưa loại trừ code.** Suýt đi sửa cloudflared tunnel; một truy vấn `public_url` đã có sẵn trong DB chứng minh tunnel vô can.
5. **Bản deploy diverge với local codebase.** Branch làm việc không có code MCP OAuth; phải `git log --all -S` mới tìm ra. Kiểm tra commit thay vì giả định branch hiện tại là đủ.
6. **Preview API của Google chặn ở tầng project, không phải scope.** Scope đúng, param đúng, token hợp lệ — vẫn 403 nếu project chưa allowlist.

## Next Steps

- [ ] Nộp form Google Workspace Developer Preview Program cho project chứa OAuth client `153552376724-…`
- [ ] Revoke token hiện tại, authorize lại bằng `client@synity.so` (Workspace account) — xác nhận không phải Gmail cá nhân
- [ ] Merge PR tech-synity/goclaw#6, deploy, rồi **gỡ Transform Rule** `d721a3df…` để tránh workaround tồn tại vĩnh viễn
- [ ] Sau khi tool call chạy được: đợi >1h rồi gọi lại — Google chỉ cấp refresh token khi authorize URL có `access_type=offline`, mà form GoClaw không có ô này. Nếu token chết sau 1h thì phải vá tiếp
- [ ] Cân nhắc đưa `gateway.public_url` vào form Config → Server trong web UI (hiện chỉ có host/port/token/owner_ids/allowed_origins)
- [ ] Dọn keychain: xoá `goclaw_api_key` (revoked), `cloudflare_api_token` (thiếu quyền), `cloudflare_account_api_token` (invalid); đổi tên `“goclaw_api_key”` sang tên không dấu ngoặc cong
