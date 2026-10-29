# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Thêm một công cụ kiểm tra uptime nữa: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Giám sát tự lưu trữ, dễ dùng: biết ngay khi website hay máy chủ của bạn gặp sự cố, mà không bị cảnh báo làm phiền.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · **Tiếng Việt** · [한국어](README.ko.md)

![Giới thiệu nhanh giao diện web của Unagikuma](docs/images/tour.gif)

## Vì sao chọn Unagikuma

- **Sẵn sàng trong khoảng 30 giây.** Tải một tệp, chạy nó và mở trình duyệt.
- **Chỉ một tệp, không cần cài thêm gì.** Chạy trên Linux, macOS, Windows và FreeBSD.
- **Kiểm tra những gì quan trọng.** Website, máy chủ, cổng, ping, DNS, chứng chỉ SSL và hạn tên miền.
- **Cảnh báo đến đúng nơi bạn dùng.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhook và nhiều kênh khác.
- **Một sự cố, không phải một trận mưa cảnh báo.** Khi chính mạng của bạn bị mất, Unagikuma nhận ra điều đó và không gửi cảnh báo cho từng mục giám sát.
- **Trang trạng thái** giúp người dùng của bạn luôn nắm được tình hình.
- **Lớn lên cùng bạn.** Kiểm tra từ nhiều địa điểm khi cần (bản xem trước).
- **Chuyển sang dễ dàng.** Mang các mục giám sát từ Uptime Kuma hoặc Nagios sang (bản xem trước).
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

## Thích dùng Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Sau đó mở http://localhost:8077. Xem **Mã thiết lập** bằng lệnh `docker logs unagikuma`.

## Tìm hiểu thêm

- [Hướng dẫn sử dụng (tiếng Anh)](docs/manual.md) — mọi thứ còn lại, từng bước một
- [Nhật ký thay đổi](CHANGELOG.md)

## Giấy phép

Apache License 2.0, xem [LICENSE](LICENSE).
