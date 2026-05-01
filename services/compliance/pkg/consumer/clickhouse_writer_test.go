package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/openguard/services/compliance/pkg/repository"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
)

type MockComplianceRepo struct {
	mock.Mock
}

func (m *MockComplianceRepo) IngestEvents(ctx context.Context, events []repository.Event) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

type MockKafkaReader struct {
	mock.Mock
}

func (m *MockKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(kafka.Message), args.Error(1)
}

func (m *MockKafkaReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	args := m.Called(ctx, msgs)
	return args.Error(0)
}

func (m *MockKafkaReader) Close() error {
	return m.Called().Error(0)
}

func TestClickHouseWriter_Flush(t *testing.T) {
	repo := new(MockComplianceRepo)
	reader := new(MockKafkaReader)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	w := &ClickHouseWriter{
		reader: reader,
		repo:   repo,
		logger: logger,
	}

	ctx := context.Background()

	// 1. Successful flush
	event := repository.Event{EventID: "ev-1", Type: "test"}
	payload, _ := json.Marshal(event)
	messages := []kafka.Message{
		{Value: payload, Offset: 100},
	}

	repo.On("IngestEvents", ctx, mock.MatchedBy(func(events []repository.Event) bool {
		return len(events) == 1 && events[0].EventID == "ev-1"
	})).Return(nil).Once()

	reader.On("CommitMessages", ctx, messages).Return(nil).Once()

	w.flush(ctx, messages)
	
	repo.AssertExpectations(t)
	reader.AssertExpectations(t)
}
