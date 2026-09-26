package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/calcron/calcron"
	"github.com/calcron/calcron/internal/server"
)

func main() {
	ctx := context.Background()
	s, err := server.New(ctx, server.Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AdminToken:  os.Getenv("CALCRON_ADMIN_TOKEN"),
		Migration:   calcron.InitialMigration,
		RetryBase:   duration("CALCRON_RETRY_BASE", time.Second),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if err := s.ListenAndServe(":" + env("PORT", "8080")); err != nil {
		log.Fatal(err)
	}
}

func duration(k string, fallback time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
