package detector

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/openguard/services/threat/pkg/alert"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHaversine(t *testing.T) {
	// NYC to London approx 5570 km
	nycLat, nycLon := 40.7128, -74.0060
	lonLat, lonLon := 51.5074, -0.1278

	dist := haversine(nycLat, nycLon, lonLat, lonLon)
	assert.InDelta(t, 5570, dist, 50) // Allow some delta for approximation

	// Same point
	assert.Equal(t, 0.0, haversine(nycLat, nycLon, nycLat, nycLon))
}

func TestImpossibleTravel_DetectionLogic(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockStore := new(MockAlertStore)
	
	d := &ImpossibleTravelDetector{
		rdb:       rdb,
		threshold: 500.0,
		windowSecs: 3600,
		logger:    slog.New(slog.NewTextHandler(os.Stdout, nil)),
		store:     mockStore,
	}

	ctx := context.Background()
	userID := "user123"

	// 1. Last login: NYC
	last := LastLogin{
		IP:        "1.1.1.1",
		Lat:       40.7128,
		Lon:       -74.0060,
		Timestamp: time.Now().Add(-10 * time.Minute),
	}

	// 2. Current login: London (Impossible)
	current := LastLogin{
		IP:        "2.2.2.2",
		Lat:       51.5074,
		Lon:       -0.1278,
		Timestamp: time.Now(),
	}

	// Mock expectations
	mockStore.On("CreateAlert", ctx, mock.MatchedBy(func(a *alert.Alert) bool {
		return a.Detector == "impossible_travel" && a.Severity == "HIGH" && a.UserID == "user123"
	})).Return(nil)

	d.detect(ctx, userID, last, current)

	// Check if alert exists in Redis (for backward compatibility)
	exists := mr.Exists("threat:travel:" + userID)
	assert.True(t, exists, "alert should be triggered for impossible travel")
	
	mockStore.AssertExpectations(t)
}
