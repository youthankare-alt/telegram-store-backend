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

	_ "github.com/syumai/workers/cloudflare/d1" // Driver D1 SQLite
	"github.com/syumai/workers"
)

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type OrderPayload struct {
	ProductID int `json:"product_id"`
}

func validateTelegramInitData(initData string, botToken string) (bool, error) {
	if initData == "" {
		return false, fmt.Errorf("init_data_empty")
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return false, err
	}

	hash := values.Get("hash")
	if hash == "" {
		return false, fmt.Errorf("hash_missing")
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

	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))
	secretKey := mac.Sum(nil)

	mac2 := hmac.New(sha256.New, secretKey)
	mac2.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(mac2.Sum(nil))

	return calculatedHash == hash, nil
}

func enableCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Telegram-Init-Data")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func main() {
	// Membaca token bot secara dinamis dari Cloudflare Secrets Environment
	botToken := os.Getenv("BOT_TOKEN")

	db, err := sql.Open("cloudflare", "DB")
	if err != nil {
		log.Fatalf("[FATAL] Gagal menghubungkan ke D1: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/products" && r.Method == "GET" {
			rows, err := db.Query("SELECT id, name, price, description, image_url FROM products")
			if err != nil {
				log.Printf("[ERROR] Gagal query produk: %v", err)
				http.Error(w, `{"error": "gagal mengambil data produk"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var products []map[string]interface{}
			for rows.Next() {
				var id int
				var name, description, imageURL string
				var price float64
				if err := rows.Scan(&id, &name, &price, &description, &imageURL); err != nil {
					log.Printf("[ERROR] Gagal scanning baris produk: %v", err)
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
			json.NewEncoder(w).Encode(products)
			return
		}

		if r.URL.Path == "/api/orders" && r.Method == "POST" {
			initData := r.Header.Get("X-Telegram-Init-Data")
			isValid, err := validateTelegramInitData(initData, botToken)
			if err != nil || !isValid {
				log.Printf("[WARN] Verifikasi gagal: %v", err)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Verifikasi initData gagal"}`))
				return
			}

			values, _ := url.ParseQuery(initData)
			var tgUser TelegramUser
			if err := json.Unmarshal([]byte(values.Get("user")), &tgUser); err != nil {
				log.Printf("[ERROR] Parsing user data gagal: %v", err)
				http.Error(w, `{"error": "user_data_invalid"}`, http.StatusBadRequest)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, `{"error": "cannot_read_body"}`, http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			var payload OrderPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, `{"error": "invalid_json"}`, http.StatusBadRequest)
				return
			}

			_, err = db.Exec(
				"INSERT INTO orders (telegram_user_id, product_id, status) VALUES (?, ?, 'PENDING')",
				tgUser.ID, payload.ProductID,
			)
			if err != nil {
				log.Printf("[ERROR] Gagal mencatat ke D1: %v", err)
				http.Error(w, `{"error": "gagal menyimpan pesanan"}`, http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"success": true, "message": "Pesanan berhasil dicatat"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "rute tidak ditemukan"}`))
	})

	workers.Serve(handler)
}
