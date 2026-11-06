# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Satu lagi alat pemantau uptime: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Pemantauan self-hosted yang mudah: langsung tahu saat situs atau server Anda mati, tanpa notifikasi berlebihan.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · **Bahasa Indonesia** · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Daftar monitor dengan nada Kertas](docs/images/monitors-paper.png)

## Mengapa Unagikuma

- **Siap dalam sekitar 30 detik.** Unduh satu file, jalankan, lalu buka browser.
- **Cukup satu file, tidak perlu memasang apa pun lagi.** Berjalan di Linux, macOS, Windows, dan FreeBSD.
- **Memeriksa hal yang penting.** Situs web, server, port, ping, DNS, sertifikat SSL, dan masa berlaku domain.
- **Peringatan di tempat Anda biasa berada.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhook, dan lainnya.
- **Satu masalah, bukan banjir peringatan.** Jika jaringan Anda sendiri yang putus, Unagikuma menyadarinya dan tidak mengirim peringatan untuk setiap monitor.
- **Halaman status** agar pengguna Anda selalu tahu kabar terbaru.
- **Tumbuh bersama Anda.** Tambahkan mesin kedua atau seluruh klaster dalam satu menit saat dibutuhkan, dengan kode bergabung dari halaman web.
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

## Satu mesin, dua mesin, sebuah klaster

> **Sekali dayung, dua tiga pulau terlampaui.** Kebanyakan alat memaksa Anda memilih: yang kecil, mudah dimulai tetapi terikat pada satu mesin, atau yang besar, bisa diperluas tetapi butuh basis data, agen, dan satu sore penuh untuk menyiapkannya. Unagikuma adalah keduanya. Mulailah dengan satu file di satu mesin; saat ingin lebih, gabungkan mesin lain. Program yang sama, tampilan yang sama, tanpa migrasi apa pun.

Unagikuma dirancang pertama-tama untuk satu mesin. Semua berjalan di sana: pemeriksaan, peringatan, halaman status, riwayat. Menambah mesin hanya menambah sudut pandang.

### Satu mesin: langsung siap

Jalankan file-nya dan tambahkan sebuah monitor. Tidak ada lagi yang perlu disiapkan: tanpa basis data, tanpa agen, tanpa server kedua. Begitulah kebanyakan orang memakainya, dan itu sudah cukup.

![Ringkasan: di mana putusnya, dari mesin ini ke setiap target](docs/images/overview.png)

### Dua mesin: masing-masing berjaga sendiri

Letakkan server kedua di tempat lain, misalnya di kantor lain atau di penyedia lain. Di server pertama buka **Pengaturan › Node › Kode gabung**; di server kedua buka **Pengaturan › Node › Gabung klaster** lalu tempel kodenya. Server kedua langsung menerima salinan konfigurasi dan segera mulai memeriksa.

- **Kedua server memeriksa dan memberi peringatan secara mandiri.** Jika salah satu mati, yang lain tetap bekerja dan tetap mengirim peringatan.
- **Tanpa menebak.** Saat dua server saling kehilangan kontak, tidak ada yang bisa tahu sisi mana yang terputus, jadi tidak ada yang menyatakan pihak lain mati. Keduanya terus memeriksa dan memberi peringatan sendiri.
- **Setiap pesan menyebut situs asalnya** (`[Tokyo] …`), sehingga Anda selalu tahu dari mana sebuah peringatan datang.

### Klaster: semua situs, satu gambaran

Tambahkan situs sebanyak yang Anda mau dengan cara yang sama. Setiap situs tetap memeriksa dan memberi peringatan sendiri, jadi kehilangan situs mana pun, termasuk server utama, tidak pernah menghentikan pemantauan. Yang ditambahkan klaster adalah konteks: setiap monitor dan setiap peringatan menampilkan apa yang sedang dilihat situs lain saat ini.

![Monitor yang gagal beserta hasil dari setiap situs](docs/images/detail-sites.png)

- **"Mati di mana-mana" atau "mati dari sini saja"?** Pengamatan dari setiap situs menunjukkan sekilas apakah sebuah layanan benar-benar mati atau hanya satu jalur jaringan yang putus. Hasil yang sudah usang ditandai.
- **Situs yang diam dilaporkan sekali saja,** setelah satu menit (bisa diatur), dengan pemberitahuan pulih saat kembali. Memulai ulang server utama tidak memicu gelombang pesan "situs tidak terjangkau".
- **Perubahan sampai ke semua situs dalam hitungan detik,** dan setiap situs dapat menyimpan monitor miliknya sendiri, seperti router di kantor itu.

![Halaman node: setiap situs, waktu pulang-pergi, dan konfigurasinya](docs/images/nodes.png)

## Banjir peringatan: satu masalah, satu pesan

Alat pemantau yang memanggil Anda seratus kali karena satu kabel putus akhirnya dibisukan, lalu tak berguna. Unagikuma memperlakukan banjir peringatan sebagai cacat yang harus dicegah:

- **Jaringan sendiri dulu.** Saat mesin ini kehilangan koneksi internetnya sendiri, Unagikuma melihat bahwa semua target gagal bersamaan dan mengirim satu pesan tentang jaringan, bukan satu per monitor. Saat jaringan kembali, monitor yang ditahan diperiksa ulang secara bertahap sebelum ada yang dilaporkan.
- **Induk sebelum anak.** Saat sebuah host mati, layanan di atasnya tidak dilaporkan satu per satu.
- **Paling banyak tiga peringatan per menit per kanal.** Sisanya dikumpulkan menjadi satu ringkasan di akhir menit, yang terparah lebih dulu. Masalah baru punya jatah sendiri sehingga tak pernah tertunda oleh pengingat, tetapi seratus layanan yang gagal bersamaan tetap datang sebagai satu ringkasan, bukan seratus pesan.
- **Kabar baik tidak pernah ditahan.** Pemulihan dan eskalasi selalu langsung dikirim: Anda tak perlu menunggu pesan yang mengatakan boleh berhenti.
- **Naik-turun diringkas.** Layanan yang hidup-mati dilaporkan sebagai satu insiden dengan hitungan.
- **Pekerjaan terencana tetap tenang.** Jendela pemeliharaan membisukan apa yang Anda umumkan akan dikerjakan.

Setiap batas dapat diatur di **Pengaturan › Parameter**.

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

Ketik kata sandi baru dua kali (tidak ditampilkan). Tambahkan `--generate` untuk mendapatkan kata sandi acak.

## Pelajari lebih lanjut

- [Panduan pengguna (dalam bahasa Inggris)](docs/manual.md) — semua hal lainnya, langkah demi langkah
- [Catatan perubahan](CHANGELOG.md)

## Lisensi

Apache License 2.0, lihat [LICENSE](LICENSE).
