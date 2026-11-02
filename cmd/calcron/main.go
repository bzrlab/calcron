package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/bzrlab/calcron/internal/server"
)

func main() {
	ctx := context.Background()
	retryBase, err := duration("CALCRON_RETRY_BASE", time.Second)
	if err != nil {
		log.Fatal(err)
	}
	retentionPeriod, err := duration("CALCRON_RETENTION", 30*24*time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	s, err := server.New(ctx, server.Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AdminToken:      os.Getenv("CALCRON_ADMIN_TOKEN"),
		Migration:       calcron.InitialMigration,
		MaxPayloadBytes: payloadLimit(),
		RetryBase:       retryBase,
		RetentionPeriod: retentionPeriod,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if err := s.ListenAndServe(":" + env("PORT", "8080")); err != nil {
		log.Fatal(err)
	}
}

func duration(k string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(k)
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", k)
	}
	return d, nil
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func payloadLimit() int64 {
	value := os.Getenv("CALCRON_MAX_PAYLOAD_BYTES")
	if value == "" {
		return 0
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit <= 0 {
		log.Fatal("CALCRON_MAX_PAYLOAD_BYTES must be a positive integer")
	}
	return limit
}
