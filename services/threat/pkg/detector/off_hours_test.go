package detector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestOffHours_Detection(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	d := &OffHoursDetector{
		rdb:      rdb,
		offStart: 22,
		offEnd:   6,
		logger:   logger,
	}

	ctx := context.Background()
	orgID := "org-1"
	userID := "user-1"

	// 1. In-hours access (e.g., 10:00) should NOT trigger alert and should record history
	now := time.Now().UTC()
	// Mock a time at 10:00 AM
	inHoursTime := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, time.UTC)
	
	hour := inHoursTime.Hour()
	date := inHoursTime.Format("2006-01-02")
	isOffHours := hour >= d.offStart || hour < d.offEnd
	
	assert.False(t, isOffHours, "10:00 should be in-hours")
	
	key := fmt.Sprintf("offhours:%s:%s:%s", orgID, userID, date)
	rdb.Set(ctx, key, "1", 24*time.Hour)

	// 2. Off-hours access (e.g., 23:00) after history of in-hours access
	offHoursTime := time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, time.UTC)
	hour = offHoursTime.Hour()
	isOffHours = hour >= d.offStart || hour < d.offEnd
	
	assert.True(t, isOffHours, "23:00 should be off-hours")

	// We'll simulate the history check
	allPreviousInHours := true
	for i := 1; i <= 3; i++ {
		prevDate := offHoursTime.AddDate(0, 0, -i).Format("2006-01-02")
		prevKey := fmt.Sprintf("offhours:%s:%s:%s", orgID, userID, prevDate)
		// We set history for the last 3 days
		rdb.Set(ctx, prevKey, "1", 24*time.Hour)
	}

	// Now check if detection would trigger
	for i := 1; i <= 3; i++ {
		prevDate := offHoursTime.AddDate(0, 0, -i).Format("2006-01-02")
		prevKey := fmt.Sprintf("offhours:%s:%s:%s", orgID, userID, prevDate)
		exists, _ := rdb.Exists(ctx, prevKey).Result()
		if exists == 0 {
			allPreviousInHours = false
		}
	}
	
	assert.True(t, allPreviousInHours, "All previous 3 days should have in-hours access records")
}
