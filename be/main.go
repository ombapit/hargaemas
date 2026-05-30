package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/robfig/cron/v3"

	_ "github.com/lib/pq"
)

//go:embed templates/index.html
var indexHTML []byte

//go:embed static
var staticFiles embed.FS

var (
	db       *sql.DB
	reDigits = regexp.MustCompile(`[^0-9]`)
)

var jakartaLoc = time.FixedZone("WIB", 7*60*60)

type GoldPrice struct {
	ID         int64     `json:"id"`
	PriceDate  string    `json:"price_date"`
	RecordedAt time.Time `json:"recorded_at"`
	Kadar      string    `json:"kadar"`
	HargaBeli  int64     `json:"harga_beli"`
}

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://hargaemas:hargaemasBismillah123!@localhost:5432/hargaemas?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	// every day 10:00 WIB (05:00 UTC)
	c := cron.New(cron.WithLocation(jakartaLoc))
	c.AddFunc("0 10 * * *", func() {
		log.Println("cron: fetching gold prices")
		if err := fetchAndStore(); err != nil {
			log.Println("cron fetch error:", err)
		}
	})
	c.Start()
	defer c.Stop()

	http.HandleFunc("/", handleIndex)
	http.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	http.HandleFunc("/robots.txt", handleRobots)
	http.HandleFunc("/sitemap.xml", handleSitemap)
	http.HandleFunc("/prices/latest", handleLatestPrices)
	http.HandleFunc("/prices", handlePrices)
	http.HandleFunc("/fetch", handleFetch)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// GET /prices/latest — data dari price_date terbaru yang ada di DB
func handleLatestPrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
		SELECT id, price_date, recorded_at, kadar, harga_beli FROM gold_prices
		WHERE price_date = (SELECT MAX(price_date) FROM gold_prices)
		ORDER BY CAST(REGEXP_REPLACE(kadar, '[^0-9]', '', 'g') AS INTEGER) DESC, kadar DESC`)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		log.Println("query error:", err)
		return
	}
	defer rows.Close()

	prices := []GoldPrice{}
	for rows.Next() {
		var p GoldPrice
		var pd time.Time
		if err := rows.Scan(&p.ID, &pd, &p.RecordedAt, &p.Kadar, &p.HargaBeli); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		p.PriceDate = pd.Format("2006-01-02")
		prices = append(prices, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prices)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: https://hargaemas.davidsuwandi.my.id/sitemap.xml\n")
}

func handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://hargaemas.davidsuwandi.my.id/</loc>
    <changefreq>daily</changefreq>
    <priority>1.0</priority>
  </url>
</urlset>`)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

// POST /fetch — manual trigger
func handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := fetchAndStore(); err != nil {
		log.Println("manual fetch error:", err)
		http.Error(w, fmt.Sprintf("fetch error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// GET /prices?date=2026-05-19
// GET /prices?from=2026-05-01&to=2026-05-19
// GET /prices?from=2026-05-01&to=2026-05-19&kadar=K24
func handlePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	var fromDate, toDate string

	if date := q.Get("date"); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			http.Error(w, "invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		fromDate = date
		toDate = date
	} else {
		fromDate = q.Get("from")
		toDate = q.Get("to")
		if fromDate == "" || toDate == "" {
			http.Error(w, "provide date=YYYY-MM-DD or from=YYYY-MM-DD&to=YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		if _, err := time.Parse("2006-01-02", fromDate); err != nil {
			http.Error(w, "invalid from format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		if _, err := time.Parse("2006-01-02", toDate); err != nil {
			http.Error(w, "invalid to format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}

	kadar := q.Get("kadar")

	var (
		rows *sql.Rows
		err  error
	)
	if kadar != "" {
		rows, err = db.Query(
			`SELECT id, price_date, recorded_at, kadar, harga_beli FROM gold_prices
			 WHERE price_date >= $1 AND price_date <= $2 AND kadar = $3
			 ORDER BY price_date DESC,
			          CAST(REGEXP_REPLACE(kadar, '[^0-9]', '', 'g') AS INTEGER) DESC,
			          kadar DESC`,
			fromDate, toDate, kadar,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, price_date, recorded_at, kadar, harga_beli FROM gold_prices
			 WHERE price_date >= $1 AND price_date <= $2
			 ORDER BY price_date DESC,
			          CAST(REGEXP_REPLACE(kadar, '[^0-9]', '', 'g') AS INTEGER) DESC,
			          kadar DESC`,
			fromDate, toDate,
		)
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		log.Println("query error:", err)
		return
	}
	defer rows.Close()

	prices := []GoldPrice{}
	for rows.Next() {
		var p GoldPrice
		var pd time.Time
		if err := rows.Scan(&p.ID, &pd, &p.RecordedAt, &p.Kadar, &p.HargaBeli); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			log.Println("scan error:", err)
			return
		}
		p.PriceDate = pd.Format("2006-01-02")
		prices = append(prices, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prices)
}

type rawPrice struct {
	Kadar     string
	HargaBeli int64
}

func fetchAndStore() error {
	prices, err := scrapeGoldPrices()
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}
	if len(prices) == 0 {
		return fmt.Errorf("no prices scraped")
	}

	today := time.Now().In(jakartaLoc).Format("2006-01-02")

	for _, p := range prices {
		_, err := db.Exec(`
			INSERT INTO gold_prices (price_date, recorded_at, kadar, harga_beli)
			VALUES ($1, NOW(), $2, $3)
			ON CONFLICT (price_date, kadar)
			DO UPDATE SET harga_beli = EXCLUDED.harga_beli, recorded_at = NOW()`,
			today, p.Kadar, p.HargaBeli,
		)
		if err != nil {
			log.Printf("upsert %s: %v", p.Kadar, err)
		}
	}

	log.Printf("stored %d prices for %s", len(prices), today)
	return nil
}

func scrapeGoldPrices() ([]rawPrice, error) {
	resp, err := http.Get("https://rajaemasindonesia.co.id/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var prices []rawPrice

	// find karat table: header contains "kadar"
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		headerText := strings.ToLower(table.Find("th, thead td").First().Text())
		if !strings.Contains(headerText, "kadar") {
			return
		}

		table.Find("tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() < 2 {
				return
			}

			kadar := strings.TrimSpace(cells.Eq(0).Text())
			priceText := strings.TrimSpace(cells.Eq(1).Text())

			// only rows starting with K (K24, K18, K10, K24*, etc.)
			if !strings.HasPrefix(kadar, "K") {
				return
			}

			cleaned := reDigits.ReplaceAllString(priceText, "")
			if cleaned == "" {
				return
			}
			harga, err := strconv.ParseInt(cleaned, 10, 64)
			if err != nil || harga == 0 {
				return
			}

			prices = append(prices, rawPrice{Kadar: kadar, HargaBeli: harga})
		})
	})

	return prices, nil
}
