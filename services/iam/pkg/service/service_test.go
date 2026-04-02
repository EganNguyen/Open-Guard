package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/stretchr/testify/assert"
)

type badTx struct{ pgx.Tx }
func (t *badTx) Rollback(ctx context.Context) error { return nil }
func (t *badTx) Commit(ctx context.Context) error { return nil }
func (t *badTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if len(sql) > 10 && sql[:6] == "SELECT" {
		return pgconn.CommandTag{}, nil // allow RLS setup 
	}
	return pgconn.CommandTag{}, errors.New("db exec failed")
}
func (t *badTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &badRow{}
}
func (t *badTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("query failed")
}

type badRow struct{}
func (r *badRow) Scan(dest ...any) error { return errors.New("query failed") }

type mockedPool struct { beginErr error }
func (m *mockedPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginErr != nil { return nil, m.beginErr }
	return &badTx{}, nil
}

func TestAuthService_Coverage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	
	// Test early begin failure
	repo := repository.New()
	svcBadDB := New(&mockedPool{beginErr: errors.New("db unreachable")}, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)
	
	_, err := svcBadDB.Register(context.Background(), RegisterInput{OrgName:"O", Email:"e@e.c", Password:"password123"})
	assert.ErrorContains(t, err, "begin tx")

	// Test inner query failures (covers much more of the functions)
	svc := New(&mockedPool{beginErr: nil}, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)

	t.Run("invalid inputs", func(t *testing.T) {
		_, err := svc.Register(context.Background(), RegisterInput{})
		assert.ErrorContains(t, err, "invalid inputs")
	})

	t.Run("register query fail", func(t *testing.T) {
		_, err := svc.Register(context.Background(), RegisterInput{OrgName:"Acme",Email:"t@t.c",Password:"12345678"})
		assert.ErrorContains(t, err, "create org") // fails at org create query
	})

	t.Run("login fail", func(t *testing.T) {
		ip, ua := "1.1.1.1", "curl"
		_, err := svc.Login(context.Background(), LoginInput{Email:"t@t.c",Password:"pass"}, &ip, &ua)
		assert.ErrorContains(t, err, "invalid credentials") // user get fails, mapped to invalid creds
	})

	t.Run("logout fail", func(t *testing.T) {
		err := svc.Logout(context.Background(), "sid", "org", "usr")
		assert.ErrorContains(t, err, "revoke session")
	})
}

func TestUserService_Coverage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	svc := New(&mockedPool{beginErr: nil}, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)

	t.Run("list users", func(t *testing.T) {
		_, _, err := svc.ListUsers(context.Background(), "org1", 0, 0)
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("get user", func(t *testing.T) {
		_, err := svc.GetUser(context.Background(), "org1", "u1")
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("create user", func(t *testing.T) {
		_, err := svc.CreateUser(context.Background(), CreateUserInput{Email:"t@t.c"})
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("update user", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), "o", "u", UpdateUserInput{})
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("delete user", func(t *testing.T) {
		err := svc.DeleteUser(context.Background(), "o", "u")
		assert.ErrorContains(t, err, "db exec failed")
	})

	t.Run("suspend user", func(t *testing.T) {
		_, err := svc.SuspendUser(context.Background(), "o", "u")
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("activate user", func(t *testing.T) {
		_, err := svc.ActivateUser(context.Background(), "o", "u")
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("list tokens", func(t *testing.T) {
		_, err := svc.ListAPITokens(context.Background(), "o", "u")
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("create token", func(t *testing.T) {
		expires := time.Now().Add(1*time.Hour)
		_, _, err := svc.CreateAPIToken(context.Background(), "o", "u", "name", nil, &expires)
		assert.ErrorContains(t, err, "query failed") // APITokenRepo create query fails
	})

	t.Run("revoke token", func(t *testing.T) {
		err := svc.RevokeAPIToken(context.Background(), "o", "t")
		assert.ErrorContains(t, err, "db exec failed")
	})

	t.Run("list sessions", func(t *testing.T) {
		_, err := svc.ListSessions(context.Background(), "o", "u")
		assert.ErrorContains(t, err, "query failed")
	})

	t.Run("revoke session", func(t *testing.T) {
		err := svc.RevokeSession(context.Background(), "o", "s")
		assert.ErrorContains(t, err, "db exec failed")
	})
}

// ---------------------------------------------
// Positive assertions (Good Pool & Tx)
// ---------------------------------------------
type goodTx struct{ pgx.Tx }
func (t *goodTx) Rollback(ctx context.Context) error { return nil }
func (t *goodTx) Commit(ctx context.Context) error { return nil }
func (t *goodTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil // always works
}
func (t *goodTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &goodRow{}
}
func (t *goodTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

type goodRow struct{}
func (r *goodRow) Scan(dest ...any) error {
	if len(dest) == 14 { // User
		*dest[0].(*string) = "u1"
		*dest[1].(*string) = "org1"
		*dest[2].(*string) = "t@t.c"
		*dest[3].(*string) = "Test User"
		hash := "$2a$04$3xvRnfuA5Fe4ykbK5QdixuIZg5oQ5lTVA7OD6x9r6uQ3k3nWMMIbq" // "password"
		*dest[4].(**string) = &hash
		*dest[5].(*string) = "active"
		*dest[6].(*bool) = false
		*dest[7].(**string) = nil
		*dest[8].(**string) = nil
		*dest[9].(*string) = "complete"
		*dest[10].(*string) = "shared"
		*dest[11].(*time.Time) = time.Now()
		*dest[12].(*time.Time) = time.Now()
		*dest[13].(**time.Time) = nil
		return nil
	}
	if len(dest) == 4 { // Org
		*dest[0].(*string) = "org1"
		*dest[1].(*string) = "Acme"
		*dest[2].(*string) = "acme"
		*dest[3].(*string) = "shared"
		return nil
	}
	if len(dest) == 11 { // Session
		// We can just ignore setting the values, returning nil is usually enough if it doesn't panic on deref
		return nil
	}
	return nil // catch all
}

type goodPool struct{}
func (m *goodPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return &goodTx{}, nil
}

func TestAuthService_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	keyring := crypto.NewJWTKeyring([]crypto.JWTKey{{Kid: "k1", Secret: "12345678901234567890123456789012", Algorithm: "HS256", Status: "active"}})

	repo := repository.New()
	svc := New(&goodPool{}, repo, nil, logger, keyring, nil, 900*time.Second, 3600*time.Second, true)

	t.Run("login success", func(t *testing.T) {
		ip, ua := "1.1.1.1", "curl"
		resp, err := svc.Login(context.Background(), LoginInput{Email:"t@t.c",Password:"password"}, &ip, &ua)
		if err != nil {
			t.Fatal(err)
		}
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Token)
	})

	t.Run("register success", func(t *testing.T) {
		resp, err := svc.Register(context.Background(), RegisterInput{OrgName:"Acme", Email:"t@t.c", Password:"password"})
		if err != nil {
			t.Fatal(err)
		}
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Token)
	})
}

func TestUserService_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	svc := New(&goodPool{}, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)

	t.Run("get user success", func(t *testing.T) {
		_, err := svc.GetUser(context.Background(), "o", "u")
		assert.NoError(t, err)
	})
	t.Run("create user success", func(t *testing.T) {
		_, err := svc.CreateUser(context.Background(), CreateUserInput{Email:"a@a.c"})
		assert.NoError(t, err)
	})
	t.Run("update user success", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), "o", "u", UpdateUserInput{})
		assert.NoError(t, err)
	})
	t.Run("delete user success", func(t *testing.T) {
		err := svc.DeleteUser(context.Background(), "o", "u")
		assert.NoError(t, err)
	})
	t.Run("suspend user success", func(t *testing.T) {
		_, err := svc.SuspendUser(context.Background(), "o", "u")
		assert.NoError(t, err)
	})
}

