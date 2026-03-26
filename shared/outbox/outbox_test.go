package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/openguard/shared/models"
)

type fakeTx struct {
	pgx.Tx
	execCalls []string
	execErr   error
}

func (f *fakeTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	f.execCalls = append(f.execCalls, sql)
	return pgconn.CommandTag{}, f.execErr
}

func TestWriter_TableName(t *testing.T) {
	w := NewWriter()
	assert.Equal(t, "outbox_records", w.getTableName())

	w.TableName = "custom_outbox"
	assert.Equal(t, "custom_outbox", w.getTableName())
}

func TestRelay_TableName(t *testing.T) {
	r := NewRelay(nil, nil)
	assert.Equal(t, "outbox_records", r.getTableName())

	r.TableName = "custom_outbox"
	assert.Equal(t, "custom_outbox", r.getTableName())
	
	r2 := &Relay{TableName: "abc"}
	assert.Equal(t, "abc", r2.getTableName())

	r3 := &Relay{TableName: "policy_outbox_records"}
	assert.Equal(t, "policy_outbox_records", r3.getTableName())
}

func TestWriter_Write(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		w := NewWriter()
		tx := &fakeTx{}
		env := models.EventEnvelope{ID: "evt123"}

		err := w.Write(context.Background(), tx, "test.topic", "key123", env)
		assert.NoError(t, err)
		assert.Len(t, tx.execCalls, 1)
		assert.Contains(t, tx.execCalls[0], "INSERT INTO outbox_records")
	})

	t.Run("db error", func(t *testing.T) {
		w := NewWriter()
		tx := &fakeTx{execErr: errors.New("db error")}
		err := w.Write(context.Background(), tx, "test.topic", "key123", models.EventEnvelope{})
		assert.ErrorContains(t, err, "db error")
	})
}

// --- Relay Mocks ---

type mockDB struct {
	OutboxDB
	beginTx    pgx.Tx
	err        error
	acquireErr error
	beginErr   error
	conn       *pgxpool.Conn
}

func (m *mockDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return m.beginTx, m.beginErr
}

func (m *mockDB) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	return nil, m.acquireErr
}

type mockProducer struct {
	OutboxProducer
	calls []string
	err   error
}

func (m *mockProducer) PublishRaw(ctx context.Context, topic string, key []byte, payload []byte) error {
	m.calls = append(m.calls, topic)
	return m.err
}

// mockCaptureProducer always returns err and records all topics published to via onPublish.
type mockCaptureProducer struct {
	err       error
	onPublish func(topic string)
}

func (m *mockCaptureProducer) PublishRaw(ctx context.Context, topic string, key []byte, payload []byte) error {
	if m.onPublish != nil {
		m.onPublish(topic)
	}
	return m.err
}


type mockRows struct {
	pgx.Rows
	data    [][]any
	index   int
	closed  bool
}

func (m *mockRows) Next() bool {
	return m.index < len(m.data)
}

func (m *mockRows) Scan(dest ...any) error {
	row := m.data[m.index]
	m.index++
	for i, val := range row {
		switch d := dest[i].(type) {
		case *string:
			*d = val.(string)
		case *[]byte:
			*d = val.([]byte)
		case *int:
			*d = val.(int)
		}
	}
	return nil
}

func (m *mockRows) Close() {
	m.closed = true
}

func (m *mockRows) Err() error { return nil }

type fullFakeTx struct {
	pgx.Tx
	queryRows pgx.Rows
	queryErr  error
	execErr   error
	committed bool
	rolled    bool
}

func (f *fullFakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.queryRows, f.queryErr
}

func (f *fullFakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.execErr
}

func (f *fullFakeTx) Commit(ctx context.Context) error   { f.committed = true; return nil }
func (f *fullFakeTx) Rollback(ctx context.Context) error { f.rolled = true; return nil }

func TestRelay_ProcessBatch(t *testing.T) {
	t.Run("db_begin_error", func(t *testing.T) {
		db := &mockDB{beginErr: errors.New("begin failed")}
		r := NewRelay(db, nil)
		err := r.processBatch(context.Background())
		assert.ErrorContains(t, err, "begin failed")
	})
	t.Run("success_delivery", func(t *testing.T) {
		rows := &mockRows{
			data: [][]any{
				{"id1", "topic1", "key1", []byte(`{"payload":1}`), 0},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}
		prod := &mockProducer{}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.True(t, tx.committed)
		assert.Len(t, prod.calls, 1)
	})

	t.Run("empty_batch", func(t *testing.T) {
		rows := &mockRows{data: [][]any{}}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}
		prod := &mockProducer{}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.Len(t, prod.calls, 0)
	})

	t.Run("kafka_error_leads_to_retry_update", func(t *testing.T) {
		rows := &mockRows{
			data: [][]any{
				{"id2", "topic2", "key2", []byte(`raw`), 0},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}
		prod := &mockProducer{err: errors.New("kafka down")}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err) // We handle kafka errors by updating DB status, so processBatch succeeds
		assert.True(t, tx.committed)
	})
	
	t.Run("db_query_error", func(t *testing.T) {
		tx := &fullFakeTx{queryErr: errors.New("query failed")}
		db := &mockDB{beginTx: tx}
		
		r := NewRelay(db, nil)
		err := r.processBatch(context.Background())
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("success_updates_status", func(t *testing.T) {
		rows := &mockRows{
			data: [][]any{
				{"id1", "topic1", "key1", []byte(`{"p":1}`), 0},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}
		prod := &mockProducer{}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.True(t, tx.committed)
	})

	t.Run("kafka_publish_error_updates_attempts", func(t *testing.T) {
		rows := &mockRows{
			data: [][]any{
				{"id_err", "topic_err", "key_err", []byte(`{"p":2}`), 2},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}
		prod := &mockProducer{err: errors.New("kafka error")}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.True(t, tx.committed)
	})
}

func TestRelay_ProcessBatch_DLQ(t *testing.T) {
	t.Run("marks_dead_and_publishes_to_dlq_after_max_attempts", func(t *testing.T) {
		// Record is already at maxAttempts-1 failures, so this attempt will hit the DLQ threshold.
		rows := &mockRows{
			data: [][]any{
				{"id_dead", "topic_dead", "key_dead", []byte(`{"event":"dlq-test"}`), maxAttempts - 1},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}

		var publishedTopics []string
		prod := &mockCaptureProducer{
			err: errors.New("kafka unavailable"),
			onPublish: func(topic string) { publishedTopics = append(publishedTopics, topic) },
		}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.True(t, tx.committed)
		// The first call is the original topic (which fails), the second is the DLQ publish.
		assert.Len(t, publishedTopics, 2, "expected original topic publish + DLQ publish")
		assert.Equal(t, dlqTopic, publishedTopics[1], "second publish must go to DLQ topic")
	})

	t.Run("does_not_publish_dlq_before_max_attempts", func(t *testing.T) {
		// Record only at attempt 1 — should not go to DLQ yet.
		rows := &mockRows{
			data: [][]any{
				{"id_retry", "topic_retry", "key_retry", []byte(`{"event":"retry-test"}`), 1},
			},
		}
		tx := &fullFakeTx{queryRows: rows}
		db := &mockDB{beginTx: tx}

		var publishedTopics []string
		prod := &mockCaptureProducer{
			err: errors.New("kafka unavailable"),
			onPublish: func(topic string) { publishedTopics = append(publishedTopics, topic) },
		}

		r := NewRelay(db, prod)
		err := r.processBatch(context.Background())

		assert.NoError(t, err)
		assert.True(t, tx.committed)
		// Only one publish attempt (to the original topic), no DLQ.
		assert.Len(t, publishedTopics, 1)
		assert.NotEqual(t, dlqTopic, publishedTopics[0])
	})
}

func TestNewRelay(t *testing.T) {
	r := NewRelay(nil, nil)
	assert.NotNil(t, r)
	assert.Equal(t, "outbox_records", r.getTableName())
}

func TestRelay_Start_AcquireError(t *testing.T) {
	db := &mockDB{acquireErr: errors.New("acquire failed")}
	r := NewRelay(db, nil)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	
	// Start is blocking, so we run it in a goroutine
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()
	
	<-done
}

func TestRelay_Start(t *testing.T) {
	db := &mockDB{}
	prod := &mockProducer{}
	r := NewRelay(db, prod)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return immediately
	r.Start(ctx)
}
