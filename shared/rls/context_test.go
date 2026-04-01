package rls

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestContext(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", OrgID(ctx))

	ctx = WithOrgID(ctx, "org-123")
	assert.Equal(t, "org-123", OrgID(ctx))
}

type fakeTx struct {
	pgx.Tx
	execCalls []string
	execArgs  [][]any
	execErr   error
}

func (f *fakeTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	f.execCalls = append(f.execCalls, sql)
	f.execArgs = append(f.execArgs, arguments)
	return pgconn.CommandTag{}, f.execErr
}

func TestSetSessionVar(t *testing.T) {
	t.Run("empty org id", func(t *testing.T) {
		tx := &fakeTx{}
		err := SetSessionVar(context.Background(), tx, "")
		assert.NoError(t, err)
		assert.Len(t, tx.execCalls, 1)
		assert.Equal(t, "SELECT set_config('app.org_id', '', false)", tx.execCalls[0])
	})

	t.Run("with org id", func(t *testing.T) {
		tx := &fakeTx{}
		err := SetSessionVar(context.Background(), tx, "org-123")
		assert.NoError(t, err)
		assert.Len(t, tx.execCalls, 1)
		assert.Equal(t, "SELECT set_config('app.org_id', $1, false)", tx.execCalls[0])
		assert.Equal(t, "org-123", tx.execArgs[0][0])
	})

	t.Run("exec error", func(t *testing.T) {
		tx := &fakeTx{execErr: errors.New("db error")}
		err := SetSessionVar(context.Background(), tx, "org-123")
		assert.ErrorContains(t, err, "db error")
	})
}
