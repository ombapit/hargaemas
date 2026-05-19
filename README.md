# Harga Emas

Scraper + API harga beli emas dari [rajaemasindonesia.co.id](https://rajaemasindonesia.co.id).

## Struktur

```
hargaemas/
├── docker-compose.yml
├── init.sql            # schema PostgreSQL
├── postgres_data/      # data DB (auto-generated, git-ignored)
└── be/
    ├── Dockerfile
    ├── go.mod
    └── main.go
```

## Cara Menjalankan

```bash
docker compose up --build
```

API berjalan di `http://localhost:8080`.

## Endpoints

### GET /prices — ambil data harga

Filter by satu tanggal:
```
GET /prices?date=2026-05-19
```

Filter by range tanggal:
```
GET /prices?from=2026-05-01&to=2026-05-19
```

Filter by tanggal + kadar:
```
GET /prices?date=2026-05-19&kadar=K24
```

**Response:**
```json
[
  {
    "id": 1,
    "price_date": "2026-05-19",
    "recorded_at": "2026-05-19T05:00:01Z",
    "kadar": "K24",
    "harga_beli": 2265000
  }
]
```

### POST /fetch — fetch manual

Trigger scraping sekarang tanpa menunggu jadwal cron.
Data hari ini akan di-replace jika sudah ada.

```bash
curl -X POST http://localhost:8080/fetch
```

## Jadwal Otomatis

Scraper berjalan otomatis setiap hari **jam 12:00 WIB (05:00 UTC)**.

## Database

PostgreSQL 16. Tabel `gold_prices`:

| Kolom        | Tipe        | Keterangan                        |
|--------------|-------------|-----------------------------------|
| `id`         | BIGSERIAL   | Primary key                       |
| `price_date` | DATE        | Tanggal harga (unique per kadar)  |
| `recorded_at`| TIMESTAMPTZ | Waktu insert/update terakhir      |
| `kadar`      | VARCHAR(20) | Kadar emas (K24, K18, K10, dst)   |
| `harga_beli` | BIGINT      | Harga beli per gram (IDR)         |

Upsert by `(price_date, kadar)` — fetch ulang hari yang sama akan update data, bukan duplikat.

## Kadar yang Di-scrape

Tabel karat kiri dari website: K24, K24*, K18, K17, K16, K14, K10, K9, K8, K6, K5.

## Environment Variables

| Variabel | Default | Keterangan |
|----------|---------|------------|
| `DB_DSN` | `postgres://hargaemas:...@localhost:5432/hargaemas?sslmode=disable` | PostgreSQL DSN |
| `PORT`   | `8080`  | Port API |
