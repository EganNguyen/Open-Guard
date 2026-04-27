package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
	Repository // Embed to satisfy interface
}

func (m *MockRepository) CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error) {
	args := m.Called(ctx, orgID, email, passwordHash, displayName, role, status)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) BeginTx(ctx context.Context) (any, error) { // Fixed to match pgx.Tx interface expectations
	args := m.Called(ctx)
	return args.Get(0), args.Error(1)
}

func (m *MockRepository) CreateOutboxEvent(ctx context.Context, tx any, orgID, topic, key string, payload []byte) error {
	args := m.Called(ctx, tx, orgID, topic, key, payload)
	return args.Error(0)
}

func (m *MockRepository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (map[string]interface{}, error) {
	args := m.Called(ctx, orgID, externalID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func TestService_RegisterUser_SagaStart(t *testing.T) {
	// This test would verify that RegisterUser correctly:
	// 1. Hashes the password
	// 2. Creates the user with 'initializing' status
	// 3. Publishes the outbox event to start the saga
}
