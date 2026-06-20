package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/syumai/workers"
	workerd1 "github.com/syumai/workers/cloudflare/d1"
)

// Representasi User Telegram dari decoded initData
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// Payload yang dikirimkan oleh Vue frontend saat melakukan pemesanan
type OrderPayload struct {
	ProductID int `json:"product_id"`
}

// Fungsi Kriptografi untuk memvalidasi tanda tangan data dari Telegram WebApp (HMAC-SHA256)
func validateTelegramInitData(initData string, botToken string) (bool, error) {
	if initData == "" {
		return false, fmt.Errorf("init_data_kosong")
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return false, err
	}

	hash := values.Get("hash")
	if hash == "" {
		return false, fmt.Errorf("hash_tidak_ditemukan")
	}

	var pairs []string
	for k, v := range values {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v[0]))
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	// Langkah 1: Buat Secret Key dari botToken dengan kunci "WebAppData"
	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))
	secretKey := mac.Sum(nil)

	// Langkah 2: Hitung hash akhir dari dataCheckString
	mac2 := hmac.New(sha256.New, secretKey)
	mac2.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(mac2.Sum(nil))

	return calculatedHash == hash, nil
}

// Penanganan Kebijakan CORS secara Modular
func enableCORS(w http.ResponseWriter, r *http.Request) bool {
	// Catatan Keamanan: Untuk produksi, Anda sangat disarankan mengubah "*" 
	// menjadi domain Cloudflare Pages Anda (contoh: "https://telegram-store-frontend.pages.dev")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Telegram-Init-Data")
	
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func main() {
	// Membaca token bot secara dinamis dari Cloudflare Environment Secrets
	botToken := os.Getenv("BOT_TOKEN")

	// Koneksi D1 SQLite menggunakan spesifikasi OpenConnector (Solusi Error 1101)
	connector, err := workerd1.OpenConnector("DB")
	if err != nil {
		log.Fatalf("[FATAL] Gagal membuat connector D1: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Jalankan pemeriksaan CORS
		if enableCORS(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// RUTE A: Mengambil Daftar Produk (GET /api/products)
		if r.URL.Path == "/api/products" && r.Method == http.MethodGet {
			rows, err := db.Query("SELECT id, name, price, description, image_url FROM products")
			if err != nil {
				log.Printf("[ERROR] Gagal melakukan query ke tabel products: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "gagal mengambil data produk"}`))
				return
			}
			defer rows.Close()

			var products []map[string]interface{}
			for rows.Next() {
				var id int
				var name, description, imageURL string
				var price float64
				if err := rows.Scan(&id, &name, &price, &description, &imageURL); err != nil {
					log.Printf("[ERROR] Gagal melakukan scan baris data produk: %v", err)
					continue
				}
				products = append(products, map[string]interface{}{
					"id":          id,
					"name":        name,
					"price":       price,
					"description": description,
					"image_url":   imageURL,
				})
			}

			// Mengembalikan array kosong jika database masih belum memiliki record produk
			if products == nil {
				products = []map[string]interface{}{}
			}

			json.NewEncoder(w).Encode(products)
			return
		}

		// RUTE B: Mencatat Pesanan Pengguna (POST /api/orders)
		if r.URL.Path == "/api/orders" && r.Method == http.MethodPost {
			// Validasi Keamanan: Ambil initData Telegram dari Custom Request Header
			initData := r.Header.Get("X-Telegram-Init-Data")
			isValid, err := validateTelegramInitData(initData, botToken)
			if err != nil || !isValid {
				log.Printf("[WARN] Percobaan transaksi dengan tanda tangan digital tidak sah. Error: %v", err)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Verifikasi initData gagal"}`))
				return
			}

			// Menguraikan string query initData untuk diekstraksi datanya
			values, err := url.ParseQuery(initData)
			if err != nil {
				log.Printf("[ERROR] Gagal menguraikan query string initData: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "format_init_data_salah"}`))
				return
			}

			userJSON := values.Get("user")
			if userJSON == "" {
				log.Printf("[WARN] Objek user kosong pada parameter initData")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "data_user_kosong"}`))
				return
			}

			var tgUser TelegramUser
			if err := json.Unmarshal([]byte(userJSON), &tgUser); err != nil {
				log.Printf("[ERROR] Gagal melakukan unmarshal JSON data user Telegram: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "format_user_invalid"}`))
				return
			}

			// Membaca payload produk yang dipesan
			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("[ERROR] Gagal membaca isi request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "gagal_membaca_payload"}`))
				return
			}
			defer r.Body.Close()

			var payload OrderPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				log.Printf("[ERROR] Format JSON payload pesanan tidak valid: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "payload_bukan_json_valid"}`))
				return
			}

			// Memasukkan pesanan baru ke database D1 SQLite
			_, err = db.Exec(
				"INSERT INTO orders (telegram_user_id, product_id, status) VALUES (?, ?, 'PENDING')",
				tgUser.ID, payload.ProductID,
			)
			if err != nil {
				log.Printf("[ERROR] Gagal menyimpan transaksi pesanan ke database D1: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "gagal mencatat transaksi ke database"}`))
				return
			}

			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"success": true, "message": "Pesanan berhasil dicatat"}`))
			return
		}

		// Rute Cadangan jika Path tidak cocok
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "rute tidak ditemukan"}`))
	})

	workers.Serve(handler)
}
