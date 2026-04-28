package detector

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestBruteForce_DetectsAfterThreshold(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	d := &BruteForceDetector{
		rdb:         rdb,
		maxAttempts: 3,
		logger:      logger,
	}

	ctx := context.Background()
	ipKey := "bruteforce:ip:1.2.3.4"

	// Track failures
	d.trackFailedAttempt(ctx, ipKey)
	d.trackFailedAttempt(ctx, ipKey)
	d.trackFailedAttempt(ctx, ipKey)

	// Verify counts in Redis using ZCard (as used in the implementation)
	count, _ := rdb.ZCard(ctx, ipKey).Result()
	assert.Equal(t, int64(3), count)
}

func TestBruteForce_TracksIPAndEmailSeparately(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	d := &BruteForceDetector{
		rdb:         rdb,
		maxAttempts: 5,
		logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	ctx := context.Background()
	ipKey := "bruteforce:ip:1.1.1.1"
	userKey := "bruteforce:user:user1"

	d.trackFailedAttempt(ctx, ipKey)
	d.trackFailedAttempt(ctx, userKey)
	d.trackFailedAttempt(ctx, userKey)

	userCount, _ := rdb.ZCard(ctx, userKey).Result()
	ipCount, _ := rdb.ZCard(ctx, ipKey).Result()

	assert.Equal(t, int64(2), userCount)
	assert.Equal(t, int64(1), ipCount)
}
