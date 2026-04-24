package detector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBruteForce_DetectsAfterThreshold(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	d := &BruteForceDetector{
		rdb:         rdb,
		logger:      slog.Default(),
		maxAttempts: 3,
	}

	ctx := context.Background()
	key := "user:test@example.com"

	// 1st attempt
	d.trackFailedAttempt(ctx, key)
	if ok, count := d.CheckRateLimit(ctx, key); !ok || count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// 2nd attempt
	d.trackFailedAttempt(ctx, key)
	// 3rd attempt -> should trigger
	d.trackFailedAttempt(ctx, key)

	if ok, _ := d.CheckRateLimit(ctx, key); ok {
		t.Error("should be rate limited")
	}

	// Check if threat was published (it writes to Redis)
	threatKey := "threat:" + key
	if !mr.Exists(threatKey) {
		t.Error("threat should be published to redis")
	}
}

func TestBruteForce_ResetsAfterWindow(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	d := &BruteForceDetector{
		rdb:         rdb,
		logger:      slog.Default(),
		maxAttempts: 3,
	}

	ctx := context.Background()
	key := "user:test@example.com"

	d.trackFailedAttempt(ctx, key)
	d.trackFailedAttempt(ctx, key)

	// Advance clock past WindowSize (5 minutes)
	mr.FastForward(6 * time.Minute)

	d.trackFailedAttempt(ctx, key)
	if ok, count := d.CheckRateLimit(ctx, key); !ok || count != 1 {
		t.Errorf("expected count 1 after window reset, got %d", count)
	}
}

func TestBruteForce_TracksIPAndEmailSeparately(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	d := &BruteForceDetector{
		rdb:         rdb,
		logger:      slog.Default(),
		maxAttempts: 3,
	}

	ctx := context.Background()
	d.trackFailedAttempt(ctx, "ip:127.0.0.1")
	d.trackFailedAttempt(ctx, "user:test@example.com")

	if ok, count := d.CheckRateLimit(ctx, "ip:127.0.0.1"); !ok || count != 1 {
		t.Errorf("expected IP count 1, got %d", count)
	}
	if ok, count := d.CheckRateLimit(ctx, "user:test@example.com"); !ok || count != 1 {
		t.Errorf("expected Email count 1, got %d", count)
	}
}
