package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	defaultDB := "../../.data/zrate.db"
	if env := os.Getenv("ZRATE_DB_PATH"); env != "" {
		defaultDB = env
	}

	dbPath := flag.String("db", defaultDB, "path to SQLite database file")
	addr := flag.String("addr", ":7022", "listen address")
	flag.Parse()

	db, err := InitDB(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/rating", handleCreateRating(db))
	mux.HandleFunc("GET /api/v1/ratings", handleListRatings(db))
	mux.HandleFunc("GET /", handleDashboard(db))

	handler := corsMiddleware(mux)

	fmt.Printf("zrate server listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
