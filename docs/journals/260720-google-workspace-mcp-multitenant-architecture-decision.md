# Kiến trúc Google Workspace MCP đa khách hàng — quyết định host + mô hình OAuth

**Date**: 2026-07-20
**Severity**: Low (decision record, chưa implement)
**Component**: MCP infrastructure / OAuth strategy / hosting
**Status**: Đã chốt hướng, chờ thực thi

Nối tiếp `260719-google-calendar-mcp-official-goclaw-setup.md`. Journal đó dừng ở chỗ dựng MCP chính chủ cho nội bộ; journal này trả lời câu hỏi lớn hơn: **triển khai cho khách hàng thì đi đường nào**.

## Bối cảnh

Sau khi setup Calendar MCP chính chủ xong (403 chờ Developer Preview), rà lại thì thấy nó **không dùng được cho khách hàng**:

1. Mỗi khách phải tự tạo OAuth client trong GCP của họ — không có app dùng chung.
2. Mỗi khách phải tự enroll **Developer Preview Program** (đăng ký theo tài khoản developer + project cụ thể, không enroll hộ hàng loạt được).
3. Điều khoản preview cấm đưa tính năng vào ứng dụng public trước GA — bán cho khách là vi phạm.

→ Self-host `taylorwilsdon/google_workspace_mcp` là đường duy nhất cho lộ trình khách hàng. Chính chủ để dùng nội bộ + theo dõi GA.

## Ba dữ kiện quyết định toàn bộ bài toán

| Câu hỏi | Trả lời | Hệ quả |
|---|---|---|
| Khách có phải Workspace hết không? | **Không** — có khách Gmail cá nhân | DWD/service account không phổ quát được |
| Cần Drive không? | **Không** — chủ yếu Calendar | **Bỏ được toàn bộ restricted scope** |
| workspace-mcp đang chạy ở đâu? | Docker trên **máy Gram** | SPOF, phải chuyển |

Dữ kiện thứ hai là bước ngoặt. Phân hạng scope của Google:

- `drive`, `drive.readonly`, Gmail → **restricted** → verification + **security assessment CASA** (audit bên thứ ba, tốn phí, làm lại định kỳ)
- `calendar`, `calendar.events`, `calendar.readonly` → **sensitive** → chỉ cần scope review
- `drive.file` → non-sensitive

Bỏ Drive ⇒ chỉ còn sensitive ⇒ **một app External của Synity chỉ phải qua verification một lần, không CASA**. Đây là thứ khiến mô hình "một app dùng chung cho mọi khách" từ bất khả thi thành khả thi.

Ngoại lệ đáng nhớ trong doc Google: app **Internal** thì miễn review kể cả restricted scope — nhưng chỉ phục vụ được user trong domain của mình, nên không dùng cho khách.

## Đọc code workspace-mcp (v1.22.0, `6e1d145`)

Kiểm tra `~/google_workspace_mcp/` xem hỗ trợ sẵn tới đâu:

| Khả năng | Trạng thái | Bật bằng |
|---|---|---|
| Multi-user OAuth 2.1 | có sẵn | `MCP_ENABLE_OAUTH21=true` |
| Stateless mode | có sẵn | `WORKSPACE_MCP_STATELESS_MODE=true` |
| Credential store ngoài FS | có sẵn (**GCS**) | `WORKSPACE_MCP_CREDENTIAL_STORE_BACKEND=gcs` + `WORKSPACE_MCP_GCS_BUCKET` |
| Service account + DWD | có sẵn | `GOOGLE_SERVICE_ACCOUNT_KEY_JSON` + `DWD_ALLOWED_DOMAINS` |
| Giới hạn tool | có sẵn | `--tools gcalendar --tool-tier core` |
| Resource-server mode | có sẵn | `EXTERNAL_OAUTH21_PROVIDER=true` |

Ba ràng buộc từ `auth/oauth_config.py`:

1. **DWD loại trừ OAuth 2.1** (`:88` raise `ValueError`). Một instance chỉ chạy một mode → muốn phục vụ cả hai kiểu khách thì phải hai instance.
2. **Một `GOOGLE_OAUTH_CLIENT_ID` mỗi instance** (`:39`). App dùng chung của Synity → một instance đủ cho mọi khách. Ngược lại, mô hình "mỗi khách một client" ⇒ mỗi khách một instance.
3. `CredentialStore` là ABC (`credential_store.py:22`) → thêm backend S3/Postgres chỉ là viết một subclass.

## Kiến trúc chốt

```
App External của Synity — chỉ calendar scopes, verification 1 lần (không CASA)
        ↓
Một instance workspace-mcp:
  MCP_ENABLE_OAUTH21=true
  WORKSPACE_MCP_STATELESS_MODE=true
  WORKSPACE_MCP_CREDENTIAL_STORE_BACKEND=gcs|s3
  --tools gcalendar --tool-tier core
        ↓
Mọi khách (Workspace lẫn Gmail cá nhân), mỗi user một token trong store
```

DWD giữ làm **tuỳ chọn riêng** cho khách enterprise không muốn từng nhân viên consent → instance riêng cho khách đó.

Giới hạn tool về `gcalendar` còn tránh được hai vấn đề ghi trong journal 19/07: `user_google_email` bắt buộc mọi tool call, và `mcpToolInlineMaxCount = 40` khiến agent rơi vào search mode khi nạp đủ 45 tool.

## So sánh nơi host

| Phương án | Chi phí | Công | Ghi chú |
|---|---|---|---|
| **VPS zuey** (đã có) | biên $0 | thấp nhất | luôn nóng, không cold start |
| **AWS Lambda** | ~$0.5–1/tháng | thấp | free tier 1M req + 400k GB-s; cold start 1–3s |
| **Cloudflare Containers** | từ $5/tháng | trung bình | disk ephemeral, vẫn phải store ngoài |
| **Cloudflare Workers** | rẻ | **cao — viết lại** | Pyodide không chạy được codebase Python này |

Đánh giá Lambda **đã được sửa lại** so với nhận định ban đầu trong hội thoại: lúc đầu kết luận "phải fork vì credentials trên disk + session in-memory", nhưng đọc code thì cả hai đã có sẵn lối ra (`WORKSPACE_MCP_STATELESS_MODE` + GCS backend). Lambda chạy được **gần như không cần fork**; muốn gọn thì viết thêm `S3CredentialStore`.

Workers thì không phải "đổi cấu hình" mà là **viết lại**: Pyodide chỉ nhận pure-Python/wheel dựng sẵn, không có filesystem bền, và mô hình request-scoped khác hẳn server sống dài.

## Thứ tự thực thi

1. **Ngay** — Docker khỏi máy Gram sang zuey VPS. Không phụ thuộc quyết định nào khác, bỏ SPOF.
2. **Song song** — nộp hồ sơ verification cho app External chỉ-calendar. Đường găng dài nhất (vài tuần), bắt đầu sớm.
3. **Sau verification** — bật OAuth21 + stateless + store ngoài; chọn Lambda hay giữ VPS. Lúc này host chỉ còn là deploy target, đổi lúc nào cũng được.

## Rủi ro cần nhớ

- **App External ở trạng thái Testing**: giới hạn 100 test user, **refresh token hết hạn sau 7 ngày** → khách phải đăng nhập lại hằng tuần. Chỉ hết khi Published + verified. Đừng demo bằng app Testing rồi tưởng sản phẩm lỗi.
- **Nếu sau này thêm Gmail**: Gmail là restricted → CASA quay lại, toàn bộ lập luận "một app External" đảo ngược. Quyết định phạm vi scope là quyết định kiến trúc, không phải quyết định tính năng.
- **DWD và OAuth21 không sống chung một instance** — nếu bán cho khách enterprise theo kiểu DWD thì phải tính chi phí vận hành nhiều instance.

## Lessons Learned

1. **Phân hạng scope quyết định kiến trúc, không phải ngược lại.** Chỉ bỏ Drive khỏi yêu cầu mà đổi được từ "CASA hằng năm + mỗi khách tự tạo app" sang "một app dùng chung, verify một lần".
2. **Đọc code trước khi kết luận về khả năng vận hành.** Nhận định "Lambda cần fork" sai vì chưa đọc `oauth_config.py` — stateless mode và GCS backend đã có sẵn từ upstream.
3. **Chọn mô hình OAuth trước, chọn nơi host sau.** Host là hệ quả của mô hình auth, không phải quyết định độc lập.

## Next Steps

- [ ] Chuyển Docker workspace-mcp từ máy Gram sang zuey VPS + cloudflared
- [ ] Xác nhận phạm vi scope cuối: chỉ Calendar, hay có Gmail (nếu có → tính lại toàn bộ)
- [ ] Chuẩn bị hồ sơ verification: homepage, privacy policy, video demo trên domain đã verify
- [ ] Đăng ký app External chỉ-calendar, nộp sensitive scope verification
- [ ] Viết `S3CredentialStore` nếu chọn Lambda (subclass `CredentialStore`)
- [ ] Đăng ký lại workspace-mcp vào GoClaw + grant agent (registration cũ đã xoá) — lưu ý nó nay chạy OAuth 2.1 có DCR, ngược với cấu hình single-user trong journal 01/07
