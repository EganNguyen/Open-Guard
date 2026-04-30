package saga

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/openguard/services/alerting/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockAlertRepo struct {
	mock.Mock
}

func (m *MockAlertRepo) Create(ctx context.Context, alert *repository.Alert) error {
	args := m.Called(ctx, alert)
	return args.Error(0)
}

func (m *MockAlertRepo) UpdateSagaStep(ctx context.Context, alertID string, step repository.SagaStep) error {
	args := m.Called(ctx, alertID, step)
	return args.Error(0)
}

func TestAlertSaga_ExecuteStep_RetriesOnFailure(t *testing.T) {
	repo := new(MockAlertRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := &AlertSaga{repo: repo, logger: logger}
	
	ctx := context.Background()
	alertID := "alert-1"
	stepName := "test-step"
	
	// 1. Success on first try
	repo.On("UpdateSagaStep", ctx, alertID, mock.MatchedBy(func(s repository.SagaStep) bool {
		return s.Step == stepName && s.Status == "completed" && s.Retries == 0
	})).Return(nil).Once()
	
	err := s.executeStep(ctx, alertID, stepName, func() error {
		return nil
	})
	assert.NoError(t, err)

	// 2. Success on 3rd try (2 failures)
	calls := 0
	repo.On("UpdateSagaStep", ctx, alertID, mock.MatchedBy(func(s repository.SagaStep) bool {
		return s.Step == stepName && s.Status == "completed" && s.Retries == 2
	})).Return(nil).Once()
	
	err = s.executeStep(ctx, alertID, stepName, func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary error")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)

	// 3. Final failure after 5 tries
	calls = 0
	repo.On("UpdateSagaStep", ctx, alertID, mock.MatchedBy(func(s repository.SagaStep) bool {
		return s.Step == stepName && s.Status == "failed" && s.Retries == 5
	})).Return(nil).Once()

	err = s.executeStep(ctx, alertID, stepName, func() error {
		calls++
		return errors.New("permanent error")
	})
	assert.Error(t, err)
	assert.Equal(t, 5, calls)
}
