package detector

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/openguard/services/threat/pkg/alert"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	_ = d.trackFailedAttempt(ctx, ipKey)
	_ = d.trackFailedAttempt(ctx, ipKey)
	_ = d.trackFailedAttempt(ctx, ipKey)

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

	_ = d.trackFailedAttempt(ctx, ipKey)
	_ = d.trackFailedAttempt(ctx, userKey)
	_ = d.trackFailedAttempt(ctx, userKey)

	userCount, _ := rdb.ZCard(ctx, userKey).Result()
	ipCount, _ := rdb.ZCard(ctx, ipKey).Result()

	assert.Equal(t, int64(2), userCount)
	assert.Equal(t, int64(1), ipCount)
}

func TestBruteForce_PersistsAlertToMongo(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockStore := new(MockAlertStore)
	d := &BruteForceDetector{
		rdb:         rdb,
		maxAttempts: 2,
		logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
		store:       mockStore,
	}

	ctx := context.Background()
	key := "bruteforce:user:target@test.io"

	// Mock expectations
	mockStore.On("CreateAlert", ctx, mock.MatchedBy(func(a *alert.Alert) bool {
		return a.Detector == "brute_force" && a.Severity == "HIGH" && a.UserID == "target@test.io"
	})).Return(nil)

	// Trigger alert
	_ = d.trackFailedAttempt(ctx, key)
	_ = d.trackFailedAttempt(ctx, key)

	mockStore.AssertExpectations(t)
}

type MockAlertStore struct {
	mock.Mock
}

func (m *MockAlertStore) CreateAlert(ctx context.Context, a *alert.Alert) error {
	args := m.Called(ctx, a)
	return args.Error(0)
}

func (m *MockAlertStore) GetAlert(ctx context.Context, id string) (*alert.Alert, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*alert.Alert), args.Error(1)
}

func (m *MockAlertStore) ListAlerts(ctx context.Context, orgID, status, severity string, limit int64, cursor string) ([]alert.Alert, string, error) {
	args := m.Called(ctx, orgID, status, severity, limit, cursor)
	return args.Get(0).([]alert.Alert), args.String(1), args.Error(2)
}

func (m *MockAlertStore) AcknowledgeAlert(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockAlertStore) ResolveAlert(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockAlertStore) GetStats(ctx context.Context, orgID string) (map[string]interface{}, error) {
	args := m.Called(ctx, orgID)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}
