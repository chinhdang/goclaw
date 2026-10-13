---
date: 2026-07-18
session: goclaw-compose-removal-design
status: design-approved
---

# Journal: 2026-07-18 — Thiết kế gỡ GoClaw Compose

## Context

Phiên brainstorm làm rõ phạm vi dọn tài nguyên GoClaw cũ trên Docker host dùng chung. Mục tiêu là loại bỏ phần còn sót của project `goclaw` mà không ảnh hưởng Docker Desktop, Dewee, Google Workspace MCP, ingress, image hay source code.

## What Happened

- Inventory xác nhận project `goclaw` không còn container, nhưng còn 4 volume và 2 network mang label ownership `com.docker.compose.project=goclaw`.
- Hai network mục tiêu không có endpoint; các workload cần bảo vệ vẫn đang chạy.
- Thiết kế chọn xoá đúng 6 tài nguyên đã xác minh bằng tên và label, kèm kiểm tra consumer ngay trước khi xoá.
- Thống nhất không dùng wildcard, `prune`, `force`, hoặc xoá image; dừng ngay nếu ownership, endpoint, consumer hay trạng thái workload bảo vệ thay đổi.
- Đây chỉ là phiên brainstorm/design. Không có lệnh xoá hay bất kỳ lệnh phá huỷ nào được thực thi.

## Reflection

Việc tách “gỡ GoClaw cũ” khỏi “gỡ Docker” giúp thu hẹp blast radius đúng vào tài nguyên orphaned. Điểm mạnh của thiết kế là dùng ownership label và hậu kiểm ID/health thay vì suy luận từ tên. Hạn chế cố ý: volume sẽ không có rollback vì người dùng đã chọn không backup, nên bước recheck trước xoá là cổng an toàn bắt buộc.

## Decisions Made

| Decision | Rationale | Impact |
|----------|-----------|--------|
| Cleanup theo exact target và Compose label | Giảm nguy cơ chạm tài nguyên dùng chung | Chỉ 4 volume và 2 network của `goclaw` nằm trong phạm vi |
| Xoá network trước, volume sau | Cho phép kiểm tra workload bảo vệ giữa hai bước | Dễ dừng sớm nếu có dấu hiệu bất thường |
| Không prune, force, wildcard hay xoá image | Các thao tác này có blast radius rộng hoặc ownership không rõ | Giữ nguyên Dewee, Google Workspace MCP, ingress và image |
| Không backup volume | Đây là quyết định phạm vi đã được chấp thuận | Việc xoá volume là vĩnh viễn, không có rollback dữ liệu |

## Next Steps

- Khi được phép thực thi, chụp baseline container ID, mount, status và health của các workload bảo vệ.
- Recheck exact name, ownership label, endpoint và consumer cho từng target.
- Xoá network không force, hậu kiểm; sau đó xoá volume không force và hậu kiểm lần cuối.
- Xác nhận không còn tài nguyên mang label project `goclaw` và không có thay đổi ngoài phạm vi.
