package utils

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DisposableTestDB struct {
	Key        string
	NameSuffix string
	Migrate    func(db *gorm.DB) error
}

type disposableTestDBState struct {
	db           *gorm.DB
	sqlDB        *sql.DB
	baseSQLDB    *sql.DB
	databaseName string
	refCount     int
}

type disposableTestDBConfig struct {
	host             string
	port             string
	username         string
	password         string
	databaseName     string
	baseDatabaseName string
	sslMode          string
}

var disposableTestDBRegistry struct {
	mu     sync.Mutex
	states map[string]*disposableTestDBState
}

func (h DisposableTestDB) Open(t *testing.T) *gorm.DB {
	t.Helper()

	cfg, ok := readDisposableTestDBConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_HOST, TEST_DATABASE_PORT, TEST_DATABASE_USERNAME, TEST_DATABASE_PASSWORD, TEST_DATABASE_NAME, and TEST_DATABASE_SSL_MODE to run repo integration tests")
	}

	disposableTestDBRegistry.mu.Lock()
	defer disposableTestDBRegistry.mu.Unlock()

	if disposableTestDBRegistry.states == nil {
		disposableTestDBRegistry.states = map[string]*disposableTestDBState{}
	}

	state := disposableTestDBRegistry.states[h.Key]
	if state == nil {
		state = &disposableTestDBState{}
		disposableTestDBRegistry.states[h.Key] = state
	}

	if state.db == nil {
		baseSQLDB, err := sql.Open("pgx", cfg.dsn(cfg.baseDatabaseName))
		require.NoError(t, err)

		databaseName := fmt.Sprintf("%s_%s_%d", cfg.databaseName, h.NameSuffix, os.Getpid())
		require.NoError(t, recreateDisposableTestDatabase(baseSQLDB, databaseName))

		db, err := gorm.Open(postgres.Open(cfg.dsn(databaseName)), &gorm.Config{})
		require.NoError(t, err)

		sqlDB, err := db.DB()
		require.NoError(t, err)

		require.NoError(t, h.Migrate(db))

		state.db = db
		state.sqlDB = sqlDB
		state.baseSQLDB = baseSQLDB
		state.databaseName = databaseName
	}

	state.refCount++
	t.Cleanup(func() {
		h.close(t)
	})

	return state.db
}

func (h DisposableTestDB) close(t *testing.T) {
	t.Helper()

	disposableTestDBRegistry.mu.Lock()
	defer disposableTestDBRegistry.mu.Unlock()

	state := disposableTestDBRegistry.states[h.Key]
	if state == nil {
		return
	}

	state.refCount--
	if state.refCount > 0 {
		return
	}

	require.NoError(t, state.sqlDB.Close())
	require.NoError(t, dropDisposableTestDatabase(state.baseSQLDB, state.databaseName))
	require.NoError(t, state.baseSQLDB.Close())

	delete(disposableTestDBRegistry.states, h.Key)
}

func readDisposableTestDBConfig() (disposableTestDBConfig, bool) {
	cfg := disposableTestDBConfig{
		host:             getDisposableTestDBEnv("TEST_DATABASE_HOST", "DATABASE_HOST"),
		port:             getDisposableTestDBEnv("TEST_DATABASE_PORT", "DATABASE_PORT"),
		username:         getDisposableTestDBEnv("TEST_DATABASE_USERNAME", "DATABASE_USERNAME"),
		password:         getDisposableTestDBEnv("TEST_DATABASE_PASSWORD", "DATABASE_PASSWORD"),
		databaseName:     getDisposableTestDBEnv("TEST_DATABASE_NAME", "DATABASE_NAME"),
		baseDatabaseName: getDisposableTestDBEnv("TEST_DATABASE_BASE_NAME", "TEST_DATABASE_NAME", "DATABASE_NAME"),
		sslMode:          getDisposableTestDBEnv("TEST_DATABASE_SSL_MODE", "DATABASE_SSL_MODE"),
	}
	return cfg, cfg.host != "" &&
		cfg.port != "" &&
		cfg.username != "" &&
		cfg.password != "" &&
		cfg.databaseName != "" &&
		cfg.baseDatabaseName != "" &&
		cfg.sslMode != ""
}

func getDisposableTestDBEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func (c disposableTestDBConfig) dsn(databaseName string) string {
	return (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.username, c.password),
		Host:   net.JoinHostPort(c.host, c.port),
		Path:   databaseName,
		RawQuery: url.Values{
			"sslmode": []string{c.sslMode},
		}.Encode(),
	}).String()
}

func recreateDisposableTestDatabase(db *sql.DB, databaseName string) error {
	if err := dropDisposableTestDatabase(db, databaseName); err != nil {
		return err
	}
	_, err := db.Exec(fmt.Sprintf(`CREATE DATABASE %s`, quoteDisposableTestDBIdentifier(databaseName)))
	return err
}

func dropDisposableTestDatabase(db *sql.DB, databaseName string) error {
	ctxDeadline := 5 * time.Second
	_, err := db.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = %s
		  AND pid <> pg_backend_pid()
	`, quoteDisposableTestDBLiteral(databaseName)))
	if err != nil {
		return err
	}

	deadline := time.Now().Add(ctxDeadline)
	for {
		_, err = db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteDisposableTestDBIdentifier(databaseName)))
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func quoteDisposableTestDBIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteDisposableTestDBLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}
