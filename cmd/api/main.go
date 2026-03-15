package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rich/notification_service/internal/notify"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := notify.NewServer(notify.NewService())
	log.Printf("notification service listening on %s", addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
