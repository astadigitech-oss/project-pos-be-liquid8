# 🚀 Backend Application

Backend application built with Go.

---

## 📌 Prerequisites

Pastikan sudah terinstall:

- Go (sesuai versi pada `go.mod`)
- Database (MySQL / PostgreSQL sesuai konfigurasi project)
- Git

Cek versi Go:

```bash
go version
```

---

# ⚙️ Installation & Setup

## 1️⃣ Setup Environment File

Buat file `.env` di root project.

Jika tersedia file contoh:

```bash
cp .env.example .env
```

Kemudian sesuaikan konfigurasi berikut:

```env
APP_ENV=development # development | production.
APP_PORT=8080

DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASS=your_password
DB_NAME=your_database
```

---

## 2️⃣ Generate JWT_SECRET_KEY dan APP_KEY

Jalankan perintah berikut:

```bash
go run ./cmd/key_generate
```

Perintah ini akan menghasilkan:

- `JWT_SECRET_KEY`
- `APP_KEY`


Pastikan tidak ada spasi tambahan atau karakter yang tidak perlu.

---

## 3️⃣ Migration Database

Untuk melihat informasi dan cara penggunaan migration:

```bash
go run ./cmd/migrate/main.go
```

Contoh menjalankan migration:

```bash
go run ./cmd/migrate/main.go -up
```

Pastikan database sudah dibuat sebelum menjalankan migration.

---

## 4️⃣ Seeder Data

Untuk melihat informasi dan cara penggunaan seeder:

```bash
go run ./cmd/seeder/main.go
```

---

# ▶️ Build the Application

Setelah semua langkah di atas selesai, lakukan build:

```bash
go build main.go
```

Jalankan menggunakan systemd atau Docker (hindari `go run` di production)

---

# ✅ Production Checklist

- Gunakan `APP_ENV=production`
- Ganti credential database
- Pastikan `JWT_SECRET_KEY` dan `APP_KEY` aman
- Jalankan dependency check:

```bash
go mod tidy
```

---

## 🛠 Troubleshooting

Jika terjadi error saat setup, pastikan:

- Konfigurasi database sudah benar
- File `.env` berada di root project
- Database sudah dibuat
- Dependency sudah terinstall dengan benar