---
title: "Brainstorm: Hợp đồng mention và quote giữa Zalo Personal, Open Lines và GoClaw"
date: 2026-06-20
status: draft
source: ck:docs/brainstorm
---

# Brainstorm hợp đồng mention và quote

## Tóm tắt điều hành

Luồng hiện tại có hai cơ chế quote độc lập nhưng cùng đi qua nội dung trả lời:

1. GoClaw dùng Bitrix inbound `MESSAGE_ID` làm `fields.replyId` để Bitrix Open Lines hiển thị liên kết trả lời trong UI.
2. Marketing Hub dùng Zalo `msgId` trong marker đầu dòng `[Name] #msgId` để tìm quote envelope trong Redis và gửi native quote qua `zca-js`.

GoClaw không hiểu ngữ nghĩa Zalo của `#digits`; nó chỉ bóc marker khỏi prompt, lưu vào
`bitrix_sender_prefix`, rồi gắn nguyên prefix lại ở đầu public reply. Vì vậy hợp đồng an toàn nhất là
giữ GoClaw như một opaque round-trip và loại bỏ điểm mơ hồ trong renderer của Marketing Hub. Đây là
brainstorm kiến trúc, **không phải kế hoạch triển khai** và **không khẳng định bất kỳ sửa đổi nào đã ship**.

## Phạm vi

- Luồng Zalo Personal inbound -> Marketing Hub -> Bitrix Open Lines -> GoClaw -> reply ngược về Zalo.
- Quyền sở hữu mention, quote, prefix, identifier và visibility public/whisper.
- Mismatch hiện tại và các lựa chọn hợp đồng ở mức thiết kế.

## Ngoài phạm vi

- Thay đổi code, schema, API hoặc rollout production.
- Zalo OA, connector khác, CRM linking, attachment recovery và participant/session isolation.
- Thiết kế lại toàn bộ identity model của Open Lines.

## Ranh giới trách nhiệm

| Thành phần | Sở hữu | Không nên sở hữu |
|---|---|---|
| Zalo Personal / `zca-js` | Phát inbound `uidFrom`, `msgId`, thread/content/mentions; nhận text, mention ranges và native quote | Bitrix IDs, Open Lines queue, phân tích marker text của GoClaw |
| Marketing Hub connector app | Chuẩn hóa inbound; lưu quote envelope; tạo external chat/message IDs; chèn marker; gọi `imconnector.send.messages`; nhận `ONIMCONNECTORMESSAGEADD`; render mention; gửi qua `zca-js` | `fields.replyId` UI của Bitrix; quyết định prompt/agent của GoClaw |
| Bitrix Open Lines | Tạo session/chat/message nội bộ; chuyển connector message tới bot/operator; phát webhook; chuyển public bot reply lại connector | Ý nghĩa Zalo `msgId`; Redis quote envelope; zca-js mention ranges |
| GoClaw Bitrix channel | Đăng ký bot; xác thực/dedup/route webhook theo `BOT_ID`; mention gate; tạo prompt; opaque prefix round-trip; chọn public/whisper; gắn Bitrix reply ID | Suy luận `#digits` là Zalo UID/msgId; tạo native Zalo quote; đổi text hiển thị của Marketing Hub |

Nguồn: GoClaw `internal/channels/bitrix24/channel.go:143-216`,
`internal/channels/bitrix24/webhook.go:309-320,351-465`,
`internal/channels/bitrix24/handle.go:108-203`; Marketing Hub
`services/zalo-personal-inbound-service.js:40-48,135-186`,
`services/queue/personal-inbound-worker.js:174-202`.

## Taxonomy identifier

| Identifier | Hệ thống gốc | Ý nghĩa / phạm vi | Không được nhầm với |
|---|---|---|---|
| `uidFrom`, Zalo user UID | Zalo | Người gửi Zalo; có thể dùng làm user target/session identity | Zalo `msgId`, Bitrix user/bot ID |
| Zalo `msgId` | Zalo | ID của một message Zalo; trong marker là quote key để tìm envelope | Zalo UID dù đều có thể là chuỗi số |
| External `chat.id` | Marketing Hub | `zpersonal_{senderId}` hoặc `zpersonal_grp_{threadId}` để Open Lines định tuyến session connector | Bitrix `CHAT_ID`/`DIALOG_ID` |
| External `message.id` | Marketing Hub | `zpersonal_msg_{msgId}` cho message connector | Bitrix `MESSAGE_ID` |
| Bitrix `MESSAGE_ID` | Bitrix | ID message inbound trong Bitrix; GoClaw dùng cho `fields.replyId` | Zalo `msgId` |
| Bitrix `CHAT_ID` | Bitrix | Numeric Open Lines chat/session reference trong payload connector | External `chat.id` |
| Bitrix `DIALOG_ID` | Bitrix | Đích chat API, thường `chatNN`; GoClaw dùng khi gửi | `CHAT_ID`, external `chat.id` |
| Bitrix bot ID | Bitrix | Bot đăng ký qua `imbot.register`; route webhook và mention gate | Zalo UID, Bitrix human user ID |
| Bitrix user ID | Bitrix | Nhân sự/operator hoặc user trong mention BBCode | Zalo UID, bot ID |

Nguồn: Marketing Hub `services/zalo-personal-inbound-service.js:43-47`,
`services/queue/personal-inbound-worker.js:101-106,178-202`; GoClaw
`internal/channels/bitrix24/events.go:58-77,197-210`,
`internal/channels/bitrix24/send_v2.go:28-46`.

## Luồng end-to-end

```mermaid
sequenceDiagram
    participant Z as "Zalo / zca-js"
    participant M as "Marketing Hub"
    participant R as "Redis quote store"
    participant O as "Bitrix Open Lines"
    participant G as "GoClaw Bitrix channel"

    Z->>M: inbound(uidFrom, msgId, threadId, content, mentions)
    M-->>R: SET quote:{connectionId}:{msgId} (TTL 6h)
    M->>O: imconnector.send.messages<br/>text starts [Name] #msgId
    O->>G: bot webhook(MESSAGE_ID, CHAT_ID, DIALOG_ID, BOT_ID, mentions)
    G->>G: auth + dedup + BOT_ID route + Open Lines mention gate
    G->>G: parse/strip prefix from prompt<br/>store bitrix_sender_prefix
    G->>G: agent generates reply without prefix semantics
    G->>O: public v2 reply<br/>prefix at absolute start + fields.replyId=MESSAGE_ID
    O->>M: ONIMCONNECTORMESSAGEADD
    M->>M: parse leading [Name] #msgId
    M->>R: GET quote:{connectionId}:{msgId}
    R-->>M: quote envelope or miss
    M->>M: render text/mentions; preserve visible contract
    M->>Z: sendMessage(content.msg, mentions, optional content.quote)
```

## Hai cơ chế quote, tuyệt đối không gộp

### 1. Bitrix UI reply

- GoClaw parse inbound Bitrix `MESSAGE_ID`, đưa vào metadata, và public v2 gửi
  `fields.replyId=<Bitrix MESSAGE_ID>`.
- Kết quả là quan hệ reply trong Bitrix UI; nó không tạo native quote Zalo.
- Whisper dùng v1 `imbot.message.add` với `SKIP_CONNECTOR=Y`, không có `replyId` và không đi ra connector.

Nguồn: GoClaw `internal/channels/bitrix24/handle.go:247-260`,
`cmd/gateway_consumer_normal.go:238-252`, `internal/channels/bitrix24/send.go:146-174`,
`internal/channels/bitrix24/send_v2.go:7-46`, `internal/channels/bitrix24/send_v1.go:7-46`.

### 2. Native Zalo quote

- Marketing Hub lưu 8-field quote envelope theo Zalo `msgId`, TTL 6 giờ, fail-open.
- Inbound group chèn `[senderLabel] #msgId` ở đầu text. GoClaw echo opaque prefix này.
- Outbound worker lấy `msgId` từ marker, tra Redis, rồi truyền envelope vào `sendText`; `zca-js`
  chỉ chuyển mention/quote payload xuống Zalo, không tự thêm `@` hay suy luận marker.
- Cache miss hoặc Redis lỗi làm reply trở thành plain send, không chặn gửi.

Nguồn: Marketing Hub `services/zalo-personal-quote-envelope-store.js:1-63`,
`services/queue/personal-inbound-worker.js:101-106`,
`services/queue/personal-outbound-worker.js:21-30,65-94,111-130`,
`services/zalo-personal-api-service.js:98-111`,
`node_modules/zca-js/dist/apis/sendMessage.js:121-142`.

## Hành vi GoClaw hiện tại

- `Start()` đăng ký bot bằng `imbot.register`, gắn callbacks và map `bot_id -> Channel`.
- Bot `TYPE=O` vẫn cần admin gắn thủ công vào Open Lines queue.
- `/bitrix24/events` parse event, kiểm tra application token, dedup theo event/message, rồi route theo `BOT_ID`.
- Open Lines chỉ qua gate khi bot được mention; Marketing Hub có thể inject bot mention từ Zalo group.
- Prefix hỗ trợ `[Name #digits]`, `[Name] #digits` và name-only trong Open Lines; prefix bị bỏ khỏi prompt,
  lưu `bitrix_sender_prefix`, truyền qua gateway metadata, rồi chèn lại ở vị trí tuyệt đối đầu public reply.
- Public v2 đi ra connector; whisper `SKIP_CONNECTOR=Y` chỉ ở nội bộ Bitrix.
- GoClaw coi `#digits` là opaque. Comment hiện nói đó là “connector user id” là gây hiểu nhầm;
  với Marketing Hub contract hiện tại, số này là Zalo `msgId`/quote key, **không phải Zalo UID**.

Nguồn: GoClaw `internal/channels/bitrix24/register.go:24-104,157-184`,
`internal/channels/bitrix24/factory.go:38-54`, `internal/channels/bitrix24/router.go:304-316`,
`internal/channels/bitrix24/webhook.go:309-320,351-465`,
`internal/channels/bitrix24/handle.go:108-203,247-260`,
`internal/channels/bitrix24/sender_prefix.go:8-77`,
`cmd/gateway_consumer_normal.go:221-252,595-603`, `internal/channels/bitrix24/send.go:78-118`.

## Hành vi Marketing Hub hiện tại

- Inbound lấy `msgId`, `uidFrom`, thread/content; lưu quote envelope và enqueue dữ liệu chuẩn hóa.
- Worker tạo marker `[senderLabel] #msgId`, external chat/message IDs và gọi `imconnector.send.messages`.
- Welcome bot được đọc từ Open Lines config (`WELCOME_BOT_ID`) để inject mention khi phù hợp.
- Webhook `ONIMCONNECTORMESSAGEADD` route connector personal; bot và operator đều có thể tới outbound sau dedup.
- Outbound parse marker đầu dòng, tra envelope, render mentions/styles, rồi gọi `zca-js`.
- `renderMentions` mới là nơi thêm `@`; `zca-js` giữ nguyên `msg` và mention ranges được truyền vào.
- Hành vi hiển thị hiện tại phải được bảo toàn trừ khi có yêu cầu thay đổi độc lập.

Nguồn: Marketing Hub `services/zalo-personal-inbound-service.js:40-48,76-123,135-186`,
`services/queue/personal-inbound-worker.js:101-106,174-202,219-243`,
`services/zalo-personal-bot-resolver-service.js:30-53`,
`handlers/webhook-bitrix-chat-handler.js:40-69`,
`services/zalo-personal-outbound-service.js:55-138`,
`services/zalo-personal-mention-render-service.js:178-192,247-264`.

## Quyết định của người dùng

> [!IMPORTANT]
> - Không đề xuất đổi `[Name]` thành `Name` hoặc bỏ `@`. Hiển thị Zalo hiện tại là đúng; visible text
>   không đổi nếu chưa có yêu cầu riêng.
> - Trong `[quote][Name] #msgId...[/quote]`, visible `#msgId` phải còn nguyên.
> - `#digits` sau external marker `[Name]` là Zalo message ID/quote key, không phải Zalo UID.

## Contract mismatch đã nhận diện, chưa sửa

Renderer hiện có regex toàn cục cho split form `[Name] #digits` và gán digits thành `uid`. Trong drop
region như `[quote]`, scanner đã consume cả `#digits`, nhánh suppress chỉ emit `Name`; test hiện còn kỳ
vọng kết quả mất `msgId`. Hành vi đó xung đột trực tiếp với quyết định giữ visible `#msgId` trong quote.

Trạng thái: **identified / not fixed**. Tài liệu này không thay đổi renderer hoặc test.

Nguồn: Marketing Hub `services/zalo-personal-mention-render-service.js:58-59,131-165,247-264`,
`tests/services/zalo-personal-mention-render-service.unit.test.mjs:322-365`.

## Các lựa chọn

| Phương án | Mô tả | Đánh giá |
|---|---|---|
| A. Giữ global split UID parsing | Tiếp tục coi `[Name] #digits` là UID ở mọi context | **Không chọn**: mơ hồ với marker Zalo `msgId`, dễ ping sai và làm mất visible quote key |
| B. Thu hẹp parser tại Marketing Hub | **Khuyến nghị**: chỉ bỏ split `[Name] #digits -> UID`; giữ combined `[Name #uid]` đã được xác minh là internal carrier; giữ nguyên visible render/`@`; trong quote region giữ `#msgId`; GoClaw tiếp tục opaque prefix round-trip | Ít thay đổi ownership nhất, làm rõ contract và không bắt GoClaw hiểu Zalo |
| C. Structured metadata | Thay text marker bằng metadata có trường sender/quote riêng xuyên qua các lớp | Hướng tương lai; rõ kiểu dữ liệu hơn nhưng phức tạp, phụ thuộc payload propagation và tương thích Open Lines |

## Rủi ro

| Rủi ro | Hệ quả | Giảm thiểu / kiểm chứng |
|---|---|---|
| Bitrix biến đổi whitespace/BBCode | Marker không match hoặc không ở đầu | Thu live payload ở cả bot/operator, có Unicode whitespace fixtures |
| Tắt split parser ảnh hưởng carrier cũ | Mention không resolve ở một số luồng | Characterization test riêng cho split external và combined internal |
| Prefix bị LLM sửa | Quote lookup miss | Giữ strip-before-prompt/reinsert-after-agent và test absolute start |
| Redis TTL/miss | Mất native quote | Giữ fail-open plain send, telemetry chỉ ID/boolean |
| Nhầm identifier số | Ping sai user hoặc quote sai message | Enforce taxonomy trong test names/log fields, không dùng tên generic `id` |
| Public/whisper lẫn nhau | Rò nội dung nội bộ ra Zalo | Test `SKIP_CONNECTOR=Y` và public v2 riêng biệt |

## Ma trận xác minh

| Case | Điểm quan sát | Kết quả mong đợi |
|---|---|---|
| Group inbound có bot mention | Zalo payload -> Open Lines webhook | `uidFrom` và `msgId` tách biệt; `MENTIONED_LIST` chứa Bitrix bot ID |
| GoClaw public reply | GoClaw outbound + connector webhook | Prefix ở absolute start; `replyId` bằng Bitrix inbound `MESSAGE_ID` |
| GoClaw whisper | Bitrix API + connector webhook | `SKIP_CONNECTOR=Y`; không có connector outbound |
| Quote envelope hit | Redis + zca-js outbound | Lookup bằng Zalo `msgId`; gửi `content.quote` native |
| Quote envelope miss/expired | Redis + zca-js outbound | Plain send thành công, không retry chỉ vì quote miss |
| `[quote][Name] #msgId...[/quote]` | Renderer characterization test | Không mention metadata; visible `#msgId` vẫn còn |
| Top-level `[Name]` | Renderer characterization test | Giữ hành vi `@Name`/mention hiện tại |
| Combined `[Name #uid]` | Cross-repo fixture | Vẫn resolve carrier nội bộ đã xác minh |
| Split `[Name] #digits` external | Cross-repo fixture | Digits được giữ như msgId/quote key, không resolve UID |

## Câu hỏi mở

1. Live `ONIMCONNECTORMESSAGEADD` có luôn bảo toàn prefix tuyệt đối đầu text cho cả bot và operator không?
2. Bitrix có biến đổi `[quote]`, newline hoặc non-breaking space khác nhau theo portal/client không?
3. Combined `[Name #uid]` hiện xuất phát chính xác từ những luồng nội bộ nào và có payload live đại diện chưa?
4. Structured metadata có thể đi xuyên Open Lines connector mà không cần thay contract công khai nào không?

## Bước tiếp theo

1. Thu và ẩn PII một bộ live payload end-to-end cho group bot reply, operator reply, whisper và quote hit/miss.
2. Viết characterization matrix xuyên hai repo cho identifier, prefix, visibility và hai quote mechanisms;
   chưa thay assertions cho tới khi payload live xác nhận boundary.
3. Đánh giá phương án B trên fixtures hiện có, đặc biệt combined carrier và drop-region visible text.
4. Sau khi chốt contract, mới lập implementation plan riêng kèm impact analysis, migration/test scope và rollback.

