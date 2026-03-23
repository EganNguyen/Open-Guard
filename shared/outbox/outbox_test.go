package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	beginTx pgx.Tx
	err     error
}

func (m *mockDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return m.beginTx, m.err
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
	t.Run("success_delivery", func(t *testing.T) {
		rows := &mockRows{
			data: [][]any{
				{"id1", "topic1", "key1", []byte(`{"payload":1}`)},
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
				{"id2", "topic2", "key2", []byte(`raw`)},
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
