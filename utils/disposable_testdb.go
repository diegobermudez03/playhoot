package utils

import (
	"context"
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
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DisposableTestDB is a database shared by every test of one test binary,
// dropped when the run ends. Packages using it must run tests through Run
// from TestMain.
type DisposableTestDB struct {
	Key        string
	NameSuffix string
	Migrate    func(db *gorm.DB) error
}

type disposableTestDBState struct {
	running      bool
	db           *gorm.DB
	sqlDB        *sql.DB
	baseSQLDB    *sql.DB
	databaseName string
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

// disposableTestDBLockID is the Postgres advisory lock that serializes
// creating a disposable database with sweeping orphaned ones, so a sweep
// never drops a database another test binary has just created but not yet
// connected to.
const disposableTestDBLockID = 7302190014

// Run runs the package's tests (m.Run) and drops the disposable database
// afterwards if any test created it. Call it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(db.Run(m)) }
func (h DisposableTestDB) Run(m *testing.M) int {
	disposableTestDBRegistry.mu.Lock()
	if disposableTestDBRegistry.states == nil {
		disposableTestDBRegistry.states = map[string]*disposableTestDBState{}
	}
	disposableTestDBRegistry.states[h.Key] = &disposableTestDBState{running: true}
	disposableTestDBRegistry.mu.Unlock()

	code := m.Run()

	disposableTestDBRegistry.mu.Lock()
	state := disposableTestDBRegistry.states[h.Key]
	delete(disposableTestDBRegistry.states, h.Key)
	disposableTestDBRegistry.mu.Unlock()

	if state != nil && state.db != nil {
		if err := state.dispose(); err != nil {
			fmt.Fprintf(os.Stderr, "dropping disposable test database %s: %v\n", state.databaseName, err)
			if code == 0 {
				code = 1
			}
		}
	}
	return code
}

// Open returns the run's disposable database, creating and migrating it on
// first use. Tests are skipped when no test database is configured.
func (h DisposableTestDB) Open(t *testing.T) *gorm.DB {
	t.Helper()

	cfg, ok := readDisposableTestDBConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_HOST, TEST_DATABASE_PORT, TEST_DATABASE_USERNAME, TEST_DATABASE_PASSWORD, TEST_DATABASE_NAME, and TEST_DATABASE_SSL_MODE to run repo integration tests")
	}

	disposableTestDBRegistry.mu.Lock()
	defer disposableTestDBRegistry.mu.Unlock()

	state := disposableTestDBRegistry.states[h.Key]
	if state == nil || !state.running {
		t.Fatalf("disposable test database %q used without Run: add `func TestMain(m *testing.M) { os.Exit(<testdb>.Run(m)) }` to this package", h.Key)
	}

	if state.db == nil {
		if err := h.create(cfg, state); err != nil {
			t.Fatalf("creating disposable test database: %v", err)
		}
	}
	return state.db
}

// create sweeps orphaned databases, then creates, connects to and migrates
// this run's database. If anything fails after the database exists, it is
// dropped again so a failed setup leaves nothing behind.
func (h DisposableTestDB) create(cfg disposableTestDBConfig, state *disposableTestDBState) error {
	ctx := context.Background()
	baseSQLDB, err := sql.Open("pgx", cfg.dsn(cfg.baseDatabaseName))
	if err != nil {
		return err
	}

	prefix := fmt.Sprintf("%s_%s_", cfg.databaseName, h.NameSuffix)
	databaseName := fmt.Sprintf("%s%d", prefix, os.Getpid())

	lockConn, err := baseSQLDB.Conn(ctx)
	if err != nil {
		baseSQLDB.Close()
		return err
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, disposableTestDBLockID); err != nil {
		baseSQLDB.Close()
		return err
	}
	defer lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, disposableTestDBLockID)

	if err := sweepOrphanedDisposableTestDatabases(baseSQLDB, prefix); err != nil {
		baseSQLDB.Close()
		return fmt.Errorf("sweeping orphaned test databases: %w", err)
	}
	if err := recreateDisposableTestDatabase(baseSQLDB, databaseName); err != nil {
		baseSQLDB.Close()
		return err
	}

	fail := func(err error) error {
		if dropErr := dropDisposableTestDatabase(baseSQLDB, databaseName); dropErr != nil {
			err = fmt.Errorf("%w (and dropping it failed: %s)", err, dropErr)
		}
		baseSQLDB.Close()
		return err
	}

	db, err := gorm.Open(postgres.Open(cfg.dsn(databaseName)), &gorm.Config{})
	if err != nil {
		return fail(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fail(err)
	}
	// Hold an open connection from here on, so the sweep of any other run
	// sees this database as in use.
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return fail(err)
	}
	if err := h.Migrate(db); err != nil {
		sqlDB.Close()
		return fail(fmt.Errorf("migrating: %w", err))
	}

	state.db = db
	state.sqlDB = sqlDB
	state.baseSQLDB = baseSQLDB
	state.databaseName = databaseName
	return nil
}

func (s *disposableTestDBState) dispose() error {
	defer s.baseSQLDB.Close()
	if err := s.sqlDB.Close(); err != nil {
		return err
	}
	return dropDisposableTestDatabase(s.baseSQLDB, s.databaseName)
}

// sweepOrphanedDisposableTestDatabases drops databases left behind by earlier
// runs that died before dropping their own (interrupted, killed, timed out):
// those with this prefix that nobody is connected to.
func sweepOrphanedDisposableTestDatabases(db *sql.DB, prefix string) error {
	rows, err := db.Query(`
		SELECT d.datname
		FROM pg_database d
		WHERE starts_with(d.datname, $1)
		  AND NOT EXISTS (SELECT 1 FROM pg_stat_activity a WHERE a.datname = d.datname)
	`, prefix)
	if err != nil {
		return err
	}
	var orphaned []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		orphaned = append(orphaned, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range orphaned {
		if err := dropDisposableTestDatabase(db, name); err != nil {
			return err
		}
	}
	return nil
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
