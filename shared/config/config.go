package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func Must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %q not set", key))
	}
	return v
}

func Default(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func MustInt(key string) int {
	v := Must(key)
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
	}
	return n
}

func DefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
	}
	return n
}

func MustDuration(key string) time.Duration {
	return time.Duration(MustInt(key)) * time.Millisecond
}

// MustJSON parses a JSON env var into dest.
func MustJSON(key string, dest any) {
	v := Must(key)
	if err := json.Unmarshal([]byte(v), dest); err != nil {
		panic(fmt.Sprintf("env var %q is not valid JSON: %v", key, err))
	}
}
