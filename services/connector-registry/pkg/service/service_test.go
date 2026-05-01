package service

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateConnector(ctx context.Context, id, orgID, name, clientSecret string, uris []string, apiKeyPrefix, apiKeyHash string) error {
	args := m.Called(ctx, id, orgID, name, clientSecret, uris, apiKeyPrefix, apiKeyHash)
	return args.Error(0)
}

func (m *MockRepository) FindByPrefix(ctx context.Context, prefix string) (map[string]interface{}, error) {
	args := m.Called(ctx, prefix)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockRepository) GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockRepository) DeleteConnector(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestService_RegisterConnector(t *testing.T) {
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewService(mockRepo, nil, logger)

	ctx := context.Background()
	id := "conn-1"
	orgID := "org-1"
	name := "Test Connector"
	uris := []string{"http://localhost"}

	mockRepo.On("CreateConnector", ctx, id, orgID, name, mock.Anything, uris, mock.Anything, mock.Anything).Return(nil)

	apiKey, err := svc.RegisterConnector(ctx, id, orgID, name, uris)
	assert.NoError(t, err)
	assert.Contains(t, apiKey, "ogk_")

	mockRepo.AssertExpectations(t)
}

func TestService_DeleteConnector_InvalidatesCache(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	mockRepo := new(MockRepository)
	svc := NewService(mockRepo, rdb, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	ctx := context.Background()
	id := "conn-1"
	prefix := "ogk_test_key"

	mockRepo.On("GetConnectorByID", ctx, id).Return(map[string]interface{}{
		"api_key_prefix": prefix,
	}, nil)
	mockRepo.On("DeleteConnector", ctx, id).Return(nil)

	// Pre-seed cache
	rdb.Set(ctx, "apikey:hash:"+prefix, "somehash", 0)

	err := svc.DeleteConnector(ctx, id)
	assert.NoError(t, err)

	// Check cache is gone
	exists, _ := rdb.Exists(ctx, "apikey:hash:"+prefix).Result()
	assert.Equal(t, int64(0), exists)

	mockRepo.AssertExpectations(t)
}

func (m *MockRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func TestService_SuspendConnector_InvalidatesCache(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	mockRepo := new(MockRepository)
	svc := NewService(mockRepo, rdb, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	ctx := context.Background()
	id := "conn-1"
	prefix := "ogk_test_key"

	mockRepo.On("GetConnectorByID", ctx, id).Return(map[string]interface{}{
		"api_key_prefix": prefix,
	}, nil)
	mockRepo.On("UpdateStatus", ctx, id, "suspended").Return(nil)

	// Pre-seed cache
	rdb.Set(ctx, "apikey:hash:"+prefix, "somehash", 0)

	err := svc.SuspendConnector(ctx, id)
	assert.NoError(t, err)

	// Check cache is gone
	exists, _ := rdb.Exists(ctx, "apikey:hash:"+prefix).Result()
	assert.Equal(t, int64(0), exists)

	mockRepo.AssertExpectations(t)
}
