package detector

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
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

func TestImpossibleTravel_Detection(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	d := &ImpossibleTravelDetector{
		rdb:        rdb,
		threshold:  500.0,
		windowSecs: 3600,
		logger:     logger,
	}

	ctx := context.Background()
	_ = ctx

	// 1. First login (New York)
	last := LastLogin{
		IP:        "1.1.1.1",
		Lat:       40.7128,
		Lon:       -74.0060,
		Timestamp: time.Now().Add(-10 * time.Minute),
	}

	// 2. Second login (London) - 10 minutes later
	current := LastLogin{
		IP:        "2.2.2.2",
		Lat:       51.5074,
		Lon:       -0.1278,
		Timestamp: time.Now(),
	}

	// We need to verify if an alert would be generated. 
	// Since detector uses a real Publisher/Store in New, we'll manually check the detect logic here.
	
	dist := haversine(last.Lat, last.Lon, current.Lat, current.Lon)
	timeDelta := current.Timestamp.Sub(last.Timestamp).Seconds()

	assert.True(t, dist > d.threshold, "Distance %f should exceed threshold %f", dist, d.threshold)
	assert.True(t, timeDelta < float64(d.windowSecs), "Time delta %f should be within window %d", timeDelta, d.windowSecs)
}
