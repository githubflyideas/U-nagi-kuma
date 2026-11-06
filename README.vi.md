# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Thêm một công cụ kiểm tra uptime nữa: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Giám sát tự lưu trữ, dễ dùng: biết ngay khi website hay máy chủ của bạn gặp sự cố, mà không bị cảnh báo làm phiền.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · **Tiếng Việt** · [한국어](README.ko.md)

![Danh sách giám sát ở tông Giấy](docs/images/monitors-paper.png)

## Vì sao chọn Unagikuma

- **Sẵn sàng trong khoảng 30 giây.** Tải một tệp, chạy nó và mở trình duyệt.
- **Chỉ một tệp, không cần cài thêm gì.** Chạy trên Linux, macOS, Windows và FreeBSD.
- **Kiểm tra những gì quan trọng.** Website, máy chủ, cổng, ping, DNS, chứng chỉ SSL và hạn tên miền.
- **Cảnh báo đến đúng nơi bạn dùng.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhook và nhiều kênh khác.
- **Một sự cố, không phải một trận mưa cảnh báo.** Khi chính mạng của bạn bị mất, Unagikuma nhận ra điều đó và không gửi cảnh báo cho từng mục giám sát.
- **Trang trạng thái** giúp người dùng của bạn luôn nắm được tình hình.
- **Lớn lên cùng bạn.** Khi cần, thêm máy thứ hai hoặc cả một cụm trong một phút, bằng mã tham gia ngay trên trang web.
- **Chuyển sang dễ dàng.** Mang các mục giám sát từ Uptime Kuma hoặc Nagios sang.
- **Nói ngôn ngữ của bạn.** Giao diện có sẵn 15 ngôn ngữ.

## Bắt đầu nhanh

**1.** Tải tệp phù hợp với hệ điều hành của bạn từ [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) rồi giải nén.

**2.** Chạy:

```sh
./unagikuma
```

Trên Windows, chạy `unagikuma.exe`.

**3.** Mở http://localhost:8077 trong trình duyệt và tạo tài khoản quản trị.

**4.** Bấm **Thêm giám sát**, nhập địa chỉ website, vậy là xong.

> Mở từ máy tính khác? Truy cập `http://<địa-chỉ-máy-chủ>:8077` và nhập **Mã thiết lập** mà chương trình hiển thị khi khởi động lần đầu.

## Một máy, hai máy, một cụm

> **Một công đôi việc.** Hầu hết các công cụ bắt bạn chọn: hoặc nhỏ gọn, dễ bắt đầu nhưng chỉ chạy trên một máy, hoặc lớn, mở rộng được nhưng cần cơ sở dữ liệu, agent và cả buổi chiều cài đặt. Unagikuma là cả hai. Bắt đầu bằng một tệp trên một máy; khi muốn nhiều hơn, chỉ cần cho các máy khác tham gia. Cùng một chương trình, cùng một giao diện, không phải chuyển đổi gì.

Unagikuma trước hết được làm cho một máy. Mọi thứ đều chạy ở đó: kiểm tra, cảnh báo, trang trạng thái, lịch sử. Thêm máy chỉ là thêm góc nhìn.

### Một máy: dùng được ngay

Chạy tệp và thêm một mục giám sát là xong. Không cần cơ sở dữ liệu, không cần agent, không cần máy chủ thứ hai. Phần lớn mọi người dùng như vậy, và chỉ một máy là đủ.

![Tổng quan: đứt ở đâu, từ máy này đến từng mục tiêu](docs/images/overview.png)

### Hai máy: mỗi máy tự canh

Đặt máy chủ thứ hai ở nơi khác, ví dụ ở văn phòng khác hoặc nhà cung cấp khác. Trên máy thứ nhất mở **Cài đặt › Nút › Mã tham gia**; trên máy thứ hai mở **Cài đặt › Nút › Tham gia cụm** rồi dán mã. Máy thứ hai nhận ngay bản sao cấu hình và bắt đầu kiểm tra luôn.

- **Hai máy kiểm tra và cảnh báo độc lập.** Nếu một máy ngừng, máy kia vẫn chạy và vẫn gửi cảnh báo.
- **Không đoán mò.** Khi hai máy mất liên lạc với nhau, không máy nào biết được bên nào bị cắt, nên không máy nào tuyên bố máy kia đã chết. Cả hai tiếp tục tự kiểm tra và cảnh báo.
- **Mỗi tin nhắn đều ghi tên địa điểm gửi** (`[Tokyo] …`), nên bạn luôn biết cảnh báo đến từ đâu.

### Một cụm: mọi địa điểm, một bức tranh

Thêm bao nhiêu địa điểm tùy ý theo cùng cách. Mỗi địa điểm vẫn tự kiểm tra và cảnh báo, nên mất bất kỳ địa điểm nào, kể cả máy chủ chính, cũng không làm ngừng giám sát. Cái cụm mang lại là bối cảnh: mỗi mục giám sát và mỗi cảnh báo đều cho thấy các địa điểm khác đang thấy gì ngay lúc này.

![Một mục giám sát đang lỗi cùng kết quả của từng địa điểm](docs/images/detail-sites.png)

- **"Sập ở mọi nơi" hay "chỉ sập khi nhìn từ đây"?** Quan sát từ mọi địa điểm cho bạn thấy ngay dịch vụ thật sự sập hay chỉ một đường mạng bị đứt. Kết quả đã cũ được đánh dấu rõ.
- **Địa điểm im lặng chỉ được báo một lần,** sau một phút (có thể chỉnh), kèm thông báo khôi phục khi trở lại. Khởi động lại máy chủ chính không gây ra một loạt tin "không liên lạc được địa điểm".
- **Thay đổi đến mọi địa điểm trong vài giây,** và mỗi địa điểm có thể giữ các mục giám sát riêng, như bộ định tuyến ở văn phòng đó.

![Trang nút: từng địa điểm, thời gian khứ hồi và cấu hình](docs/images/nodes.png)

## Lũ cảnh báo: một sự cố, một tin nhắn

Công cụ giám sát gọi bạn cả trăm lần vì một sợi cáp đứt rồi sẽ bị tắt tiếng, và trở nên vô dụng. Unagikuma coi lũ cảnh báo là lỗi cần ngăn chặn:

- **Mạng của chính mình trước.** Khi chính máy này mất kết nối internet, Unagikuma nhận ra mọi mục tiêu cùng lỗi một lúc và chỉ gửi một tin về mạng, thay vì mỗi mục giám sát một tin. Khi mạng trở lại, các mục bị giữ lại được kiểm tra lại dần dần trước khi báo cáo.
- **Cha trước, con sau.** Khi một máy chủ sập, các dịch vụ trên nó không bị báo từng cái một.
- **Mỗi kênh tối đa ba cảnh báo mỗi phút.** Phần vượt quá được gom thành một bản tóm tắt cuối phút, nghiêm trọng nhất lên đầu. Sự cố mới có hạn mức riêng nên không bao giờ bị nhắc nhở làm chậm, nhưng một trăm dịch vụ cùng lỗi vẫn chỉ đến thành một bản tóm tắt, không phải một trăm tin.
- **Tin tốt không bao giờ bị giữ lại.** Thông báo khôi phục và leo thang luôn được gửi ngay: bạn không phải chờ tin báo rằng có thể dừng tay.
- **Chập chờn được tóm tắt.** Dịch vụ lúc lên lúc xuống được báo thành một sự cố kèm số lần.
- **Công việc theo kế hoạch được giữ yên lặng.** Khung bảo trì tắt cảnh báo cho những gì bạn đã báo trước là sẽ làm.

Mọi giới hạn đều chỉnh được ở **Cài đặt › Tham số**.

## Chuyển từ Uptime Kuma

Dừng Uptime Kuma, rồi xem trước (không thay đổi gì):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Nếu mọi thứ ổn, nhập vào:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

Báo cáo cho biết những gì đã nhập, những gì cần xem lại và những gì không nhập được, kèm cách xử lý. Báo cáo cũng được lưu trong `data/import-reports/`. Chạy lại chỉ thêm những gì mới.

## Chuyển từ Nagios

Chỉ đến `nagios.cfg` và xem trước (không thay đổi gì):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Nếu mọi thứ ổn, nhập vào:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

Báo cáo cho biết những gì đã nhập, những gì cần xem lại và những gì không nhập được, kèm cách xử lý. Báo cáo cũng được lưu trong `data/import-reports/`. Chạy lại chỉ thêm những gì mới.

## Quên mật khẩu?

Trên máy đang chạy Unagikuma:

```sh
./unagikuma reset-password admin
```

Nhập mật khẩu mới hai lần (không hiển thị). Thêm `--generate` để nhận một mật khẩu ngẫu nhiên.

## Tìm hiểu thêm

- [Hướng dẫn sử dụng (tiếng Anh)](docs/manual.md) — mọi thứ còn lại, từng bước một
- [Nhật ký thay đổi](CHANGELOG.md)

## Giấy phép

Apache License 2.0, xem [LICENSE](LICENSE).
