# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Satu lagi alat pemantau uptime: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Pemantauan self-hosted yang mudah: langsung tahu saat situs atau server Anda mati, tanpa notifikasi berlebihan.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · **Bahasa Indonesia** · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Tur singkat antarmuka web Unagikuma](docs/images/tour.gif)

## Mengapa Unagikuma

- **Siap dalam sekitar 30 detik.** Unduh satu file, jalankan, lalu buka browser.
- **Cukup satu file, tidak perlu memasang apa pun lagi.** Berjalan di Linux, macOS, Windows, dan FreeBSD.
- **Memeriksa hal yang penting.** Situs web, server, port, ping, DNS, sertifikat SSL, dan masa berlaku domain.
- **Peringatan di tempat Anda biasa berada.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhook, dan lainnya.
- **Satu masalah, bukan banjir peringatan.** Jika jaringan Anda sendiri yang putus, Unagikuma menyadarinya dan tidak mengirim peringatan untuk setiap monitor.
- **Halaman status** agar pengguna Anda selalu tahu kabar terbaru.
- **Tumbuh bersama Anda.** Periksa dari beberapa lokasi saat dibutuhkan (pratinjau).
- **Mudah pindah.** Bawa monitor Anda dari Uptime Kuma atau Nagios.
- **Berbicara dalam bahasa Anda.** Antarmuka tersedia dalam 15 bahasa.

## Mulai cepat

**1.** Unduh file untuk sistem Anda dari [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) lalu ekstrak.

**2.** Jalankan:

```sh
./unagikuma
```

Di Windows, jalankan `unagikuma.exe`.

**3.** Buka http://localhost:8077 di browser dan buat akun admin Anda.

**4.** Klik **Tambah monitor**, masukkan alamat situs web, selesai.

> Membukanya dari komputer lain? Buka `http://<alamat-server>:8077` dan masukkan **Token penyiapan** yang ditampilkan program saat pertama kali dijalankan.

## Lebih suka Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Lalu buka http://localhost:8077. **Token penyiapan** bisa dilihat dengan `docker logs unagikuma`.

## Pindah dari Uptime Kuma

Hentikan Uptime Kuma, lalu lihat dulu (tidak ada yang diubah):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Jika sudah sesuai, impor:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

Laporan menampilkan apa yang diimpor, apa yang perlu dicek, dan apa yang tidak bisa diimpor, beserta langkah yang perlu dilakukan. Laporan juga disimpan di `data/import-reports/`. Menjalankannya lagi hanya menambahkan yang baru.

## Pindah dari Nagios

Arahkan ke `nagios.cfg` dan lihat dulu (tidak ada yang diubah):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Jika sudah sesuai, impor:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

Laporan menampilkan apa yang diimpor, apa yang perlu dicek, dan apa yang tidak bisa diimpor, beserta langkah yang perlu dilakukan. Laporan juga disimpan di `data/import-reports/`. Menjalankannya lagi hanya menambahkan yang baru.

## Lupa kata sandi?

Di mesin tempat Unagikuma berjalan:

```sh
./unagikuma reset-password admin
```

Ketik kata sandi baru dua kali (tidak ditampilkan). Tambahkan `--generate` untuk mendapatkan kata sandi acak. Dengan Docker:

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

## Pelajari lebih lanjut

- [Panduan pengguna (dalam bahasa Inggris)](docs/manual.md) — semua hal lainnya, langkah demi langkah
- [Catatan perubahan](CHANGELOG.md)

## Lisensi

Apache License 2.0, lihat [LICENSE](LICENSE).
