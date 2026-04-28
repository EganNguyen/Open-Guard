package detector

import (
	"testing"

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
