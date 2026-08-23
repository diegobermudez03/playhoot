package utils

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type testDBServicer struct {
	db *gorm.DB
}

func (s testDBServicer) GetDB() *gorm.DB {
	return s.db
}

type txContextKey struct{}

var (
	errTestCallback = errors.New("callback failed")
	errTestBegin    = errors.New("begin failed")
	errTestCommit   = errors.New("commit failed")
)

func TestRunInDBTransaction_CommitsAndReturnsCallbackResult(t *testing.T) {
	recorder := &recordingDriver{}
	db := openRecordingGormDB(t, recorder)

	ctx := context.WithValue(context.Background(), txContextKey{}, "request-1")

	result, err := RunInDBTransaction(ctx, testDBServicer{db: db}, func(callbackCtx context.Context, tx *gorm.DB) (string, error) {
		if callbackCtx != ctx {
			t.Fatal("callback received a different context")
		}
		if tx == nil {
			t.Fatal("callback received nil transaction")
		}
		return "created", nil
	})
	if err != nil {
		t.Fatalf("RunInDBTransaction returned error: %v", err)
	}
	if result != "created" {
		t.Fatalf("result = %q, want %q", result, "created")
	}

	assertEvents(t, recorder.events(), []string{"begin", "commit"})
	if got := recorder.beginContextValue(); got != "request-1" {
		t.Fatalf("begin context value = %v, want %q", got, "request-1")
	}
}

func TestRunInDBTransaction_RollsBackAndReturnsZeroValueWhenCallbackFails(t *testing.T) {
	recorder := &recordingDriver{}
	db := openRecordingGormDB(t, recorder)

	result, err := RunInDBTransaction(context.Background(), testDBServicer{db: db}, func(context.Context, *gorm.DB) (string, error) {
		return "partial result", errTestCallback
	})
	if !errors.Is(err, errTestCallback) {
		t.Fatalf("error = %v, want callback error", err)
	}
	if result != "" {
		t.Fatalf("result = %q, want zero value", result)
	}

	assertEvents(t, recorder.events(), []string{"begin", "rollback"})
}

func TestRunInDBTransaction_DoesNotCallCallbackWhenBeginFails(t *testing.T) {
	recorder := &recordingDriver{beginErr: errTestBegin}
	db := openRecordingGormDB(t, recorder)

	callbackCalled := false
	result, err := RunInDBTransaction(context.Background(), testDBServicer{db: db}, func(context.Context, *gorm.DB) (string, error) {
		callbackCalled = true
		return "unexpected", nil
	})
	if !errors.Is(err, errTestBegin) {
		t.Fatalf("error = %v, want begin error", err)
	}
	if !strings.Contains(err.Error(), "opening DB transaction") {
		t.Fatalf("error = %q, want opening context", err.Error())
	}
	if callbackCalled {
		t.Fatal("callback was called after begin failed")
	}
	if result != "" {
		t.Fatalf("result = %q, want zero value", result)
	}

	assertEvents(t, recorder.events(), []string{"begin"})
}

func TestRunInDBTransaction_ReturnsZeroValueWhenCommitFails(t *testing.T) {
	recorder := &recordingDriver{commitErr: errTestCommit}
	db := openRecordingGormDB(t, recorder)

	result, err := RunInDBTransaction(context.Background(), testDBServicer{db: db}, func(context.Context, *gorm.DB) (int, error) {
		return 42, nil
	})
	if !errors.Is(err, errTestCommit) {
		t.Fatalf("error = %v, want commit error", err)
	}
	if !strings.Contains(err.Error(), "committing transaction") {
		t.Fatalf("error = %q, want committing context", err.Error())
	}
	if result != 0 {
		t.Fatalf("result = %d, want zero value", result)
	}

	assertEvents(t, recorder.events(), []string{"begin", "commit"})
}

func openRecordingGormDB(t *testing.T, recorder *recordingDriver) *gorm.DB {
	t.Helper()

	driverName := registerRecordingDriver(recorder)
	sqlDB, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open sql DB: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close sql DB: %v", err)
		}
	})

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm DB: %v", err)
	}
	return db
}

func assertEvents(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

var recordingDriverRegistry struct {
	sync.Mutex
	next int
}

func registerRecordingDriver(recorder *recordingDriver) string {
	recordingDriverRegistry.Lock()
	defer recordingDriverRegistry.Unlock()

	recordingDriverRegistry.next++
	name := fmt.Sprintf("playhoot_recording_driver_%d", recordingDriverRegistry.next)
	sql.Register(name, recorder)
	return name
}

type recordingDriver struct {
	mu               sync.Mutex
	recordedEvents   []string
	recordedBeginCtx any
	beginErr         error
	commitErr        error
}

func (d *recordingDriver) Open(string) (driver.Conn, error) {
	return &recordingConn{driver: d}, nil
}

func (d *recordingDriver) record(event string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recordedEvents = append(d.recordedEvents, event)
}

func (d *recordingDriver) events() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	events := make([]string, len(d.recordedEvents))
	copy(events, d.recordedEvents)
	return events
}

func (d *recordingDriver) beginContextValue() any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.recordedBeginCtx
}

type recordingConn struct {
	driver *recordingDriver
}

func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not implemented")
}

func (c *recordingConn) Close() error {
	return nil
}

func (c *recordingConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *recordingConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	c.driver.mu.Lock()
	c.driver.recordedEvents = append(c.driver.recordedEvents, "begin")
	c.driver.recordedBeginCtx = ctx.Value(txContextKey{})
	err := c.driver.beginErr
	c.driver.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return &recordingTx{driver: c.driver}, nil
}

func (c *recordingConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return emptyRows{}, nil
}

type recordingTx struct {
	driver *recordingDriver
}

func (tx *recordingTx) Commit() error {
	tx.driver.record("commit")
	return tx.driver.commitErr
}

func (tx *recordingTx) Rollback() error {
	tx.driver.record("rollback")
	return nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string {
	return nil
}

func (emptyRows) Close() error {
	return nil
}

func (emptyRows) Next([]driver.Value) error {
	return io.EOF
}
