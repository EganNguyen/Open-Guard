package saga

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
)

type MockReader struct {
	mock.Mock
}

func (m *MockReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(kafka.Message), args.Error(1)
}

func (m *MockReader) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockUpdater struct {
	mock.Mock
}

func (m *MockUpdater) UpdateUserStatus(ctx context.Context, userID, status string) error {
	args := m.Called(ctx, userID, status)
	return args.Error(0)
}

func TestConsumer_HandleProvisioningEvents(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	t.Run("Handle Success Event", func(t *testing.T) {
		mockReader := new(MockReader)
		mockUpdater := new(MockUpdater)
		consumer := &Consumer{
			reader: mockReader,
			svc:    mockUpdater,
			logger: logger,
		}

		ctx, cancel := context.WithCancel(context.Background())
		
		eventData, _ := json.Marshal(map[string]string{
			"event":   "user.scim.provisioned",
			"user_id": "user-123",
		})

		mockReader.On("ReadMessage", mock.Anything).Return(kafka.Message{Value: eventData}, nil).Once()
		mockReader.On("ReadMessage", mock.Anything).Return(kafka.Message{}, context.Canceled)
		mockUpdater.On("UpdateUserStatus", mock.Anything, "user-123", "active").Return(nil)

		go func() {
			cancel() // Stop after processing
		}()

		consumer.Start(ctx)

		mockUpdater.AssertExpectations(t)
	})

	t.Run("Handle Failure Event", func(t *testing.T) {
		mockReader := new(MockReader)
		mockUpdater := new(MockUpdater)
		consumer := &Consumer{
			reader: mockReader,
			svc:    mockUpdater,
			logger: logger,
		}

		ctx, cancel := context.WithCancel(context.Background())
		
		eventData, _ := json.Marshal(map[string]string{
			"event":   "user.provisioning.failed",
			"user_id": "user-456",
		})

		mockReader.On("ReadMessage", mock.Anything).Return(kafka.Message{Value: eventData}, nil).Once()
		mockReader.On("ReadMessage", mock.Anything).Return(kafka.Message{}, context.Canceled)
		mockUpdater.On("UpdateUserStatus", mock.Anything, "user-456", "provisioning_failed").Return(nil)

		go func() {
			cancel()
		}()

		consumer.Start(ctx)

		mockUpdater.AssertExpectations(t)
	})
}
