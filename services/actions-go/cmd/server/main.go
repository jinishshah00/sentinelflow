package main

import (
    "log"
    "net/http"
    "os"
    "time"

    "github.com/jinishshah00/sentinelflow/internal/shared"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        shared.WriteJSON(w, http.StatusOK, map[string]any{
            "service": "actions-go",
            "status":  "ok",
            "time":    time.Now().UTC(),
        })
    })
    addr := ":" + getenv("PORT", "8080")
    log.Printf("actions-go listening on %s", addr)
    if err := http.ListenAndServe(addr, mux); err != nil {
        log.Fatal(err)
    }
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
