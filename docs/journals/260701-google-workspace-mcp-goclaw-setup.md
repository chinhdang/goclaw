# Google Workspace MCP Setup for GoClaw (mai / fox-spirit)

**Date**: 2026-07-01  
**Severity**: Medium  
**Component**: MCP infrastructure / agent tooling  
**Status**: Resolved

## What Happened

Set up `taylorwilsdon/google_workspace_mcp` v3.4.2 as a GoClaw MCP server for agents `mai` và `fox-spirit` trên remote instance `https://goclaw-dev.synity.so`. Toàn bộ quá trình gồm: cài server local, expose qua Cloudflare Tunnel, đăng ký vào GoClaw, và debug tại sao server bị evict khỏi pool liên tục trước khi agents có thể dùng.

## The Brutal Truth

Mất kha khá thời gian debug vì cái lỗi DCR settings không có error message rõ ràng — GoClaw chỉ evict server khỏi pool trong im lặng với `mcp.pool.server_evicted`. Không có "DCR handshake failed" hay gì tương tự. Đây là kiểu lỗi mà bạn ngồi nhìn logs không hiểu tại sao tools không xuất hiện, trong khi root cause là một config field không nên có mặt ở đó.

## Technical Details

### Infrastructure

- Cloned `taylorwilsdon/google_workspace_mcp` → `~/google_workspace_mcp/`
- `.env` file chứa: `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET` (từ macOS Keychain), `WORKSPACE_MCP_PORT=8000`, `OAUTHLIB_INSECURE_TRANSPORT=1`
- Start command: `uv run main.py --tool-tier core --transport streamable-http --single-user`
- Cloudflare Tunnel: `localhost:8000` → `https://google-ws.synity.so/mcp`
- macOS LaunchAgents: `so.synity.workspace-mcp.plist` + `so.synity.cloudflared-workspace.plist`

### GoClaw MCP Registration

```
POST /v1/mcp/servers
{
  "name": "google-workspace",
  "transport": "streamable-http",
  "url": "https://google-ws.synity.so/mcp",
  "tool_prefix": "gws",
  "timeout_sec": 60
}
```

Grants: `POST /v1/mcp/servers/{id}/grants/agent` cho `mai` và `fox-spirit`.

### Bug: OAuth DCR Settings Blocking Pool Connection

**Symptom:** `mai` báo 21 tools loaded (3 servers khác), google-workspace vắng mặt. Logs lặp đi lặp lại:
```
mcp.pool.server_evicted: google-workspace
```
Không có `mcp.server.connected_via_pool` nào cho google-workspace.

**Root cause:** Initial MCP config có `settings.oauth = {use_dcr: true, auth_type: "oauth", grant_type: "pkce"}`. Deployed GoClaw hiểu đây là OAuth 2.1 DCR mode → thực hiện DCR handshake trước `pool.Acquire`. workspace-mcp ở `--single-user` mode không hỗ trợ DCR → GoClaw fail im lặng, evict server.

**Key evidence:** `mcp.oauth_tokens_purged_on_config_change: oauth_changed=true` xuất hiện trong logs sau config update — deployed GoClaw có event này (không có trong local codebase, deployed version khác).

**Fix:** `PUT /v1/mcp/servers/{id}` với `settings: {require_user_credentials: false}` (bỏ oauth field hoàn toàn).

**Verification:** `/v1/mcp/servers/{id}/tools` trả về 45 tools ngay sau fix.

### Google OAuth First-Use Flow

- workspace-mcp v3.4.2 yêu cầu `user_google_email` argument trên mọi tool call (multi-user capable dù ở single-user mode)
- Credentials lưu tại: `~/.google_workspace_mcp/credentials/{email}.json`
- OAuth redirect URI `http://localhost:8000/oauth2callback` phải được đăng ký trong Google Cloud Console
- Flow: gọi tool với email → workspace-mcp trả về auth URL → mở browser → grant → callback nhận code → credentials saved
- Hoàn thành cho: `client@synity.so` — verified `list_calendars` trả về Primary + Bitrix24 CRM System calendars

### Tool Search Mode

- GoClaw `mcpToolInlineMaxCount = 40`: khi tổng MCP tools > 40, chuyển sang search mode
- `mai`: 21 tools hiện tại + 45 gws tools = 66 → search mode active
- `mai` dùng `mcp_tool_search` meta-tool để tìm `gws_*` tools theo tên khi cần

## What We Tried

1. Đăng ký MCP server với default settings (bao gồm oauth DCR config từ template) → server bị evict liên tục
2. Kiểm tra network connectivity tới `https://google-ws.synity.so/mcp` từ server → OK
3. Xem logs deployed GoClaw → thấy `mcp.pool.server_evicted` nhưng không có error detail
4. Phát hiện `mcp.oauth_tokens_purged_on_config_change` → trace về oauth settings
5. Remove oauth field, set `require_user_credentials: false` → fixed

## Root Cause Analysis

Deployed GoClaw có OAuth 2.1 DCR support (không có trong local codebase). Khi `settings.oauth.use_dcr = true` được set, GoClaw cố thực hiện DCR registration trước khi pool connection được thiết lập. workspace-mcp `--single-user` mode không expose DCR endpoint → handshake fail → server evicted. Không có error log nào từ phía GoClaw về DCR failure cụ thể — chỉ là silent eviction.

Lesson: deployed version có features không có trong local code. Config templates không nên blindly copy oauth DCR settings.

## Lessons Learned

1. **Đừng bao giờ set `oauth.use_dcr` vào MCP server settings** trừ khi biết chắc GoClaw và target server đều hỗ trợ OAuth 2.1 DCR. workspace-mcp single-user mode không hỗ trợ.
2. **Luôn test bằng `/v1/mcp/servers/{id}/tools`** ngay sau đăng ký để verify pool connection trước khi chuyển sang agent testing. Nếu endpoint trả về empty list → server chưa connected.
3. **workspace-mcp tool calls luôn cần `user_google_email`** — agents phải được instruct email của user cần thao tác. Không có default.
4. **`mcp.pool.all_user_connections_evicted`** trong deployed GoClaw = OAuth settings thay đổi, tokens bị purge. Event này không có trong local codebase → deployed version diverge.
5. **LaunchAgents cho tunnel + MCP server phải start đúng thứ tự**: workspace-mcp phải listen trước khi cloudflared start forward requests. Plist `RunAtLoad` + dependency ordering.

## Next Steps

- [ ] Test `mai` và `fox-spirit` gọi `gws_list_emails`, `gws_search_emails` với `user_google_email=client@synity.so`
- [ ] Document `user_google_email` requirement trong agent system prompt hoặc context file để agents không phải hỏi
- [ ] Monitor LaunchAgents sau reboot — verify auto-restart hoạt động đúng
- [ ] Nếu cần thêm Google accounts: chạy OAuth flow tương tự với email mới, credentials tự lưu tại `~/.google_workspace_mcp/credentials/`
- [ ] Xem xét nâng `mcpToolInlineMaxCount` nếu search mode gây friction cho agents
