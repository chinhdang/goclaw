## auto Contact nằm ở channel adapter, gọi chung vào ContactCollector.EnsureContact(...).

Core:
• internal/store/contact_collector.go
  - ContactCollector.EnsureContact(...)
  - cache seen TTL 30m, rồi gọi store upsert
• internal/store/contact_store.go
  - interface ContactStore.UpsertContact(...)
  - unique logic theo (tenant_id, channel_type, sender_id, thread_id)
• internal/store/pg/channel_contacts.go
  - PGContactStore.UpsertContact(...)
• internal/store/sqlitestore/contacts.go
  - SQLiteContactStore.UpsertContact(...)

Theo platform:

• Telegram
  - internal/channels/telegram/handlers.go
  - hàm chính: handleMessage(...), processReadyContext(...)
  - tạo contact:
    - user sender: senderID/userID
    - group: chatIDStr
    - topic: chatIDStr + threadID

• Discord
  - internal/channels/discord/handler.go
  - hàm chính: handleMessageCreate(...), handleMentionedMessage(...)
  - tạo contact user trong group/direct bằng senderID

• Slack
  - internal/channels/slack/utils.go
    - processMessage(...) / helper path gọi contact
  - internal/channels/slack/handlers.go
    - mention/group handling
  - tạo contact user bằng userID/senderID

• Bitrix24
  - internal/channels/bitrix24/handle.go
  - hàm chính: handleMessage(...)
  - tạo contact:
    - user sender: senderID
    - group chat: chatID
  - có enrich tên:
    - internal/channels/bitrix24/contact_enrich.go
    - resolveContactName(...)

• Feishu/Lark
  - internal/channels/feishu/bot.go
    - handleMessageContext(...)
  - internal/channels/feishu/feishu.go
    - member sync path
  - tạo contact user bằng SenderID/open_id hoặc MemberID

• Zalo Personal
  - internal/channels/zalo/personal/handlers.go
  - hàm chính:
    - handleDirectMessage(...)
    - handleGroupMessage(...)
  - tạo contact user bằng senderID, group message vẫn peerKind "group"

• Zalo OA / native cũ
  - internal/channels/zalo/zalo.go
  - hàm chính:
    - handleTextMessage(...)
    - handleImageMessage(...)
  - em chưa thấy gọi EnsureContact ở file này trong scan nhanh — chủ yếu xử lý DM sender/chat.

• WhatsApp
  - internal/channels/whatsapp/inbound.go
  - hàm chính: handleMessage(...)
  - tạo contact user bằng senderID

Tóm tắt 1 câu: muốn special-case Openlines thì chỗ đẹp nhất là trong channel adapter trước khi gọi EnsureContact, generate senderID synthetic riêng cho customer thật, ví dụ openline:<lineChatID>:zalo:<realUID>, chỉ áp dụng platform đó, không đụng core.

## auto seeding USER.md nằm riêng ở bootstrap/user profile, không phải Contact.

Core flow:
• internal/agent/loop_context.go
  - setup mỗi request gọi:
    - channelMeta := l.buildChannelMeta(req)
    - setup := l.getOrCreateUserSetup(...)
• internal/agent/loop_utils.go
  - buildChannelMeta(req)
    - lấy ChannelType, SenderName, defaultTimezone
  - getOrCreateUserSetup(...)
    - gọi ensureUserProfile(...)
    - rồi gọi seedUserFiles(...)
    - cache theo userID trong l.userSetups

Callback wiring:
• cmd/gateway_callbacks.go
  - buildEnsureUserProfile(...)
    - gọi AgentStore.GetOrCreateUserProfile(...)
  - buildSeedUserFiles(...)
    - gọi bootstrap.SeedUserFiles(...)
  - buildBootstrapCleanup(...)
    - xóa BOOTSTRAP.md

Seed implementation:
• internal/bootstrap/seed_store.go
  - SeedUserFiles(...) là hàm chính
  - open agent seed per-user:
    - AGENTS.md
    - SOUL.md
    - IDENTITY.md
    - USER.md
    - BOOTSTRAP.md
    - CAPABILITIES.md
    - AGENTS_CORE.md
    - AGENTS_TASK.md
  - predefined agent seed per-user:
    - USER.md
    - BOOTSTRAP.md
  - nếu user đã có file và skipIfAnyExist=true thì không seed lại
  - predefined agent có USER.md agent-level thì dùng nó làm seed cho per-user USER.md
  - BOOTSTRAP_PREDEFINED.md dùng cho predefined agent

Auto skip bootstrap / prefill:
• internal/bootstrap/seed_store.go
  - shouldSkipBootstrap(meta *ChannelMeta)
  - hiện tại chỉ Pancake được auto prefill nếu có DisplayName
  - buildPrefilledUser(meta)
    - tạo sẵn USER.md
    - skip BOOTSTRAP.md

Storage:
• internal/store/pg/agents_context.go
  - SetUserContextFile(...)
  - GetUserContextFiles(...)
  - DeleteUserContextFile(...)
  - MigrateUserDataOnMerge(...)
• internal/store/sqlitestore/agents_context.go
  - cùng nhóm hàm trên cho SQLite

User merge liên quan USER.md:
• internal/store/pg/agents_context.go
  - MigrateUserDataOnMerge(...)
• internal/store/sqlitestore/agents_context.go
  - MigrateUserDataOnMerge(...)
• Dùng để migrate user_context_files, profile/override/memory khi merge user ids.

Tóm lại:
• Contact: auto tạo ở từng channel qua EnsureContact(...)
• USER.md: tạo theo req.UserID trong agent loop qua SeedUserFiles(...)
• Muốn special Openlines cho USER.md thì phải đảm bảo req.UserID được normalize thành synthetic real customer id trước khi vào getOrCreateUserSetup, ví dụ openlines:<lineID>:zalo:<realUID>; còn chỉ sửa Contact thì chưa đủ.

## mối quan hệ giữa GoClaw Contact vs USER.md

Contact và USER.md là 2 lớp khác nhau:

1. Contact = “người/nhóm mà channel nhìn thấy”
Nó là dữ liệu CRM-ish trong GoClaw.

Ví dụ:
• Telegram user id 620478615
• Zalo uid abc123
• Bitrix staff id 42
• Openlines line customer id line-999
• Group chat id -100xxx

Contact dùng để:
• hiển thị trong Contacts UI
• biết sender đến từ platform nào
• merge nhiều contact thành 1 tenant user
• phục vụ permission / resolve identity
• audit, search, mapping cross-channel

Nó được tạo bởi channel adapter, kiểu:
EnsureContact(channelType, channelInstance, senderID, userID, displayName, ...)


2. USER.md = “hồ sơ riêng của agent về user đó”
Nó là context file cho agent đọc.

Ví dụ:
# USER.md
- Name: Chinh
- Timezone: Asia/Ho_Chi_Minh
- Preferences: thích nói thẳng, ghét Zalo nhưng khách ở đâu thì theo đó


USER.md dùng để:
• agent nhớ cách nói chuyện với user
• lưu preference cá nhân
• bootstrap/onboarding lần đầu
• làm context cho agent khi user đó chat

Nó được tạo theo req.UserID trong agent loop, không trực tiếp theo Contact row.

Mối quan hệ thực tế

Flow đại khái:

Channel message
  ↓
sender_id / chat_id / peer_kind
  ↓
1) ContactCollector tạo Contact
  ↓
2) Agent loop dùng req.UserID để seed/load USER.md


Hai cái có thể liên quan qua identity resolution / merge:

Contact.sender_id
  ↓ merge vào
Tenant User
  ↓ resolve thành
req.UserID
  ↓
USER.md riêng cho user đó


Quan trọng nhất

• Contact không tự động = USER.md
• USER.md không phải mỗi contact luôn có
• Contact là “định danh ngoài đời/platform”
• USER.md là “context cá nhân mà agent dùng”
• Merge contact đúng nghĩa là: nhiều account/platform của cùng một người thật → một tenant user → một USER.md hợp nhất.

Case Openlines của anh

Nếu tất cả customer trong group bị Openlines đẩy lên cùng một senderID, thì:

• Contact layer sẽ chỉ thấy 1 contact
• Agent loop cũng dễ seed/load 1 USER.md
• Dù trong text có [Name] #uid, agent có thể trả lời đúng người, nhưng hệ thống identity/permission/context vẫn đang coi là một người

Muốn đẹp thì phải tạo synthetic identity trước khi vào Contact + USER.md, ví dụ:

senderID = openlines:<lineChatID>:zalo:<realZaloUID>
req.UserID = openlines:<lineChatID>:zalo:<realZaloUID>
displayName = realName
chatID = lineChatID
peerKind = group


Khi đó:
• Contacts UI tách từng customer
• USER.md tách từng customer
• permission/merge về tenant user vẫn đúng
• reply routing vẫn dùng [Name] #uid hoặc metadata riêng

## Cách parsing Zalo uid trong Bitrix Oplenlines message trong GoClaw

data[PARAMS][CHAT_ID]=4878
data[PARAMS][DIALOG_ID]=chat4878
data[PARAMS][CHAT_ENTITY_ID]=synity_zalo_personal|20|zpersonal_grp_2793212409613162623|960
data[PARAMS][FROM_USER_ID]=960
data[PARAMS][MESSAGE]=[Chinh #3393421773556008363]: ...
data[USER][ID]=960
data[USER][IS_CONNECTOR]=Y


Kết luận nhanh

Bitrix/Openlines đang gửi FROM_USER_ID = 960, nhưng đây có vẻ là connector/customer proxy user của Bitrix, không phải real Zalo user từng người trong group.

Real Zalo-ish identity lại nằm trong body:

[Chinh #3393421773556008363]:


=> 3393421773556008363 chính là thứ nên dùng để tách contact/user trong case Zalo group.

Nên normalize như này

Từ payload:

CHAT_ID = 4878
CHAT_ENTITY_ID = synity_zalo_personal|20|zpersonal_grp_2793212409613162623|960
MESSAGE = [Chinh #3393421773556008363]: ...


Parse ra:

realName = Chinh
realZaloUID = 3393421773556008363
zaloGroupID = zpersonal_grp_2793212409613162623
bitrixChatID = 4878


Rồi set synthetic identity:

senderID = openlines:zalo-personal:zpersonal_grp_2793212409613162623:3393421773556008363


Hoặc ngắn hơn:

senderID = openlines:chat4878:zalo:3393421773556008363


Em thích bản đầu hơn vì chat4878 là Bitrix internal id, còn zpersonal_grp_... gần source thật hơn.

Contact nên tạo

channel_type = bitrix24/openlines
sender_id = openlines:zalo-personal:zpersonal_grp_2793212409613162623:3393421773556008363
thread_id/chat_id = 4878
display_name = Chinh
peer_kind = group
external_uid = 3393421773556008363


Agent request cũng phải dùng y chang

Quan trọng nhất:

req.UserID = openlines:zalo-personal:zpersonal_grp_2793212409613162623:3393421773556008363
req.SenderName = Chinh
req.ChatID = 4878
req.PeerKind = group


Nếu chỉ Contact dùng UID này mà req.UserID vẫn là 960 hoặc chat4878 thì USER.md vẫn bị gộp. Lại thành “bên ngoài phân thân, bên trong một não” — hơi dị 😄

Regex parse message

Case này parse được bằng pattern kiểu:

^\[(.+?)\s+#([0-9A-Za-z_-]+)\]:\s*(.*)$


Với message:

[Chinh #3393421773556008363]: @Ngọc Thúy Synity alo em có người yêu chưa


ra:

name = Chinh
uid = 3393421773556008363
text = @Ngọc Thúy Synity alo em có người yêu chưa


Fallback nếu parse fail

Nếu message không có format [Name #uid]: thì fallback về hiện tại:

senderID = bitrix_user:960


hoặc:

senderID = openlines:chat4878:from:960


Chốt: metadata đủ rồi, không cần sửa core. Sửa đẹp nhất ở Bitrix Openlines adapter: trước khi EnsureContact và trước khi build RunRequest, parse MESSAGE_ORIGINAL/MESSAGE, lấy #uid, rồi override sender identity.

## Một Zalo UID (SenderID) ở nhiều group khác nhau

Nếu cùng real senderID/Zalo UID nhưng chat ở nhiều group khác nhau thì phải quyết định theo semantics anh muốn:

Option A — tách theo từng group
Dùng key gồm group + user:

senderID = openlines:zalo-personal:<groupID>:<realZaloUID>


Ví dụ:

openlines:zalo-personal:zpersonal_grp_A:3393421773556008363
openlines:zalo-personal:zpersonal_grp_B:3393421773556008363


Kết quả:
• cùng Chinh nhưng ở group A/B là 2 contact khác nhau
• USER.md cũng tách theo từng group
• an toàn nhất cho permission/context group
• tránh đem ký ức group này qua group kia

Cái này hợp nếu group context quan trọng.

Option B — gom cùng người trên mọi group
Dùng key chỉ theo real UID:

senderID = openlines:zalo-personal:<realZaloUID>


Kết quả:
• cùng Chinh ở nhiều group là 1 contact
• USER.md dùng chung
• nhớ preference cá nhân tốt hơn
• nhưng dễ leak context giữa group nếu agent memory/context không scoped kỹ

Cái này hợp nếu muốn CRM xem đó là một người thật duy nhất.

Option C — đẹp nhất: 2 lớp
Em vote cái này:

Contact identity/person:
  externalUserID = zalo:3393421773556008363

Conversation participant identity:
  participantID = openlines:zalo-personal:<groupID>:3393421773556008363

USER.md key:
  userID = participantID


Tức là:
• CRM/contact có thể biết “đây cùng một người”
• nhưng agent context/USER.md vẫn tách theo group
• sau này nếu muốn merge người thật thì merge ở contact/tenant user layer, không ép agent nhớ lẫn.

Trong code thực dụng hiện tại thì làm nhanh:

req.UserID / senderID = openlines:zalo-personal:<groupID>:<realZaloUID>
threadID/chatID = bitrixChatID hoặc groupID
metadata.real_zalo_uid = <realZaloUID>
metadata.source_group_id = <groupID>


Chốt: đừng dùng mỗi real UID làm req.UserID trong group nếu chưa chắc isolation, vì rất dễ thành “Chinh ở group khách A” đem context qua “Chinh ở group khách B”. Superman cũng không cứu nổi vụ leak memory đâu 😄