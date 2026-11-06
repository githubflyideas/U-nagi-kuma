# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Thêm một công cụ kiểm tra uptime nữa: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

**Một công đôi việc:** một tệp trên một máy là dùng được ngay; muốn nhiều hơn thì cho các máy khác tham gia để thành một cụm. Cùng một chương trình, không phải chuyển đổi gì.

Giám sát tự lưu trữ, dễ dùng: biết ngay khi website hay máy chủ của bạn gặp sự cố, mà không bị cảnh báo làm phiền.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · **Tiếng Việt** · [한국어](README.ko.md)

![Danh sách giám sát ở tông Giấy](docs/images/monitors-paper.png)

## Vì sao chọn Unagikuma

- **Sẵn sàng trong khoảng 30 giây.** Tải một tệp, chạy nó và mở trình duyệt.
- **Chỉ một tệp, không cần cài thêm gì.** Chạy trên Linux, macOS, Windows và FreeBSD.
- **Kiểm tra những gì quan trọng.** Website, máy chủ, cổng, ping, DNS, chứng chỉ SSL và hạn tên miền.
- **Cảnh báo đến đúng nơi bạn dùng.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhook và nhiều kênh khác.
- **Trang trạng thái** giúp người dùng của bạn luôn nắm được tình hình.
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

- **Một máy:** chạy tệp, thêm một mục giám sát là xong. Không cần cơ sở dữ liệu, không cần agent.
- **Hai máy:** dán mã tham gia ở **Cài đặt › Nút**. Mỗi máy tự kiểm tra và cảnh báo, nên một máy ngừng thì cảnh báo vẫn không gián đoạn.
- **Một cụm:** thêm bao nhiêu địa điểm tùy ý. Mỗi cảnh báo đều cho thấy từng địa điểm đang thấy gì, để phân biệt "sập ở mọi nơi" với "chỉ sập khi nhìn từ đây".

![Một mục giám sát đang lỗi cùng kết quả của từng địa điểm](docs/images/detail-sites.png)

## Không lũ cảnh báo

- **Mạng của chính bạn bị đứt?** Chỉ một tin về mạng, không phải mỗi mục giám sát một tin.
- **Máy chủ sập?** Các dịch vụ trên nó không bị báo từng cái một.
- **Mỗi kênh tối đa ba cảnh báo mỗi phút;** phần còn lại đến thành một bản tóm tắt. Thông báo khôi phục luôn được gửi ngay.

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
