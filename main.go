package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diegobermudez03/playhoot/api"
	"github.com/diegobermudez03/playhoot/game/usecases/checkplayability"
	"github.com/diegobermudez03/playhoot/identity/usecases/authorize"
	"github.com/diegobermudez03/playhoot/identity/usecases/createguest"
	"github.com/diegobermudez03/playhoot/orchestrator/playlifecycle"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/usecases/ownership"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/liveconnection"
	sessionmigration "github.com/diegobermudez03/playhoot/play/session/migration"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	"github.com/diegobermudez03/playhoot/play/session/usecases/checkadminjoin"
	"github.com/diegobermudez03/playhoot/play/session/usecases/checkplayerjoin"
	"github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess"
	"github.com/diegobermudez03/playhoot/play/session/usecases/loadruntimemanifest"
	"github.com/diegobermudez03/playhoot/play/session/usecases/resolvejoincode"
	"github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type envVariables struct {
	Environment      string
	DatabaseHost     string
	DatabasePort     string
	DatabaseUsername string
	DatabasePassword string
	DatabaseName     string
	DatabaseSSLMode  string
	HTTPPort         string

	// WebSocketAllowedOrigins are extra origin hosts browser pages may open
	// live connections from. Optional.
	WebSocketAllowedOrigins []string

	// GCSProjectID and GCSBucket name the object storage of game version
	// content. Optional: without them every storage call fails. An emulator
	// is selected with STORAGE_EMULATOR_HOST.
	GCSProjectID string
	GCSBucket    string

	// Sandbox limits for this instance; optional, zero takes the defaults.
	SandboxLimits sandbox.HostLimits
}

// shutdownTimeout bounds how long in-flight requests get to finish once the
// process is asked to stop. Cloud Run sends SIGTERM and kills the container
// 10 seconds later, so this leaves room to close the database afterwards.
const shutdownTimeout = 8 * time.Second

func main() {
	// A sandbox context worker is a re-exec of this binary: it must serve its
	// context and exit before anything else (environment, database) runs.
	if sandbox.RunAsWorkerIfRequested() {
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run starts the application and blocks until it stops. It returns instead
// of exiting so deferred cleanup (closing the database) always runs.
func run() error {
	envVars, err := readEnvVariables()
	if err != nil {
		return fmt.Errorf("reading environment variables: %w", err)
	}

	db, err := openPostgres(envVars)
	if err != nil {
		return fmt.Errorf("opening PostgreSQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("getting PostgreSQL connection: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("closing PostgreSQL connection: %v", err)
		}
	}()

	if err := PostgresMigrate(db); err != nil {
		return fmt.Errorf("running PostgreSQL migrations: %w", err)
	}
	store, closeStore, err := storage.Open(context.Background(), envVars.GCSProjectID, envVars.GCSBucket)
	if err != nil {
		return fmt.Errorf("opening object storage: %w", err)
	}
	defer func() {
		if err := closeStore(); err != nil {
			log.Printf("closing object storage: %v", err)
		}
	}()

	if !strings.EqualFold(envVars.Environment, "production") {
		if err := PostgresSeed(db); err != nil {
			return fmt.Errorf("seeding PostgreSQL test data: %w", err)
		}
		if envVars.GCSBucket != "" {
			if err := sessionmigration.SeedObjects(context.Background(), store); err != nil {
				return fmt.Errorf("seeding object storage test data: %w", err)
			}
		}
	}

	sandboxHost := sandbox.NewHost(envVars.SandboxLimits)
	defer func() {
		if err := sandboxHost.Close(); err != nil {
			log.Printf("stopping sandbox contexts: %v", err)
		}
	}()

	sessionLifecycle := sessionlifecycle.New(db, store)
	playLifecycle := playlifecycle.New(checkplayability.New(), sessionLifecycle)
	liveConnections := liveconnection.New(checkadminjoin.New(db), checkplayerjoin.New(db), loadruntimemanifest.New(db), sessionLifecycle, sessionLifecycle, frontendaccess.New(db, store), sandboxHost, unwiredReplay{})
	// Closed before the sandbox, deferred later: controllers stop first, so
	// shutting down never fails a running game.
	defer liveConnections.Close()
	server := api.NewServer(
		authorize.New(),
		createguest.New(),
		playLifecycle,
		ownership.New(),
		liveConnections,
		liveConnections,
		resolvejoincode.New(db),
		envVars.WebSocketAllowedOrigins,
	)

	httpServer := &http.Server{
		Addr:              ":" + envVars.HTTPPort,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", httpServer.Addr)
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		// The server stopped on its own (for example, the port was taken).
		return fmt.Errorf("serving HTTP: %w", err)
	case <-stopCtx.Done():
	}

	// A second signal while shutting down kills the process immediately.
	stop()
	log.Printf("shutting down, waiting up to %s for in-flight requests", shutdownTimeout)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	// The HTTP server stops first so no new live connection is admitted;
	// it does not track hijacked (WebSocket) connections, which the API
	// server closes itself.
	httpErr := httpServer.Shutdown(shutdownCtx)
	liveErr := server.Shutdown(shutdownCtx)
	if httpErr != nil {
		return fmt.Errorf("shutting down HTTP server: %w", httpErr)
	}
	if liveErr != nil {
		return fmt.Errorf("closing live connections: %w", liveErr)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving HTTP: %w", err)
	}
	log.Printf("stopped")
	return nil
}

func readEnvVariables() (*envVariables, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ENVIRONMENT")), "production") {
		if err := godotenv.Load(); err != nil {
			return nil, fmt.Errorf("loading .env: %w", err)
		}
	}

	envVars := &envVariables{
		Environment:      strings.TrimSpace(os.Getenv("ENVIRONMENT")),
		DatabaseHost:     strings.TrimSpace(os.Getenv("DATABASE_HOST")),
		DatabasePort:     strings.TrimSpace(os.Getenv("DATABASE_PORT")),
		DatabaseUsername: strings.TrimSpace(os.Getenv("DATABASE_USERNAME")),
		DatabasePassword: os.Getenv("DATABASE_PASSWORD"),
		DatabaseName:     strings.TrimSpace(os.Getenv("DATABASE_NAME")),
		DatabaseSSLMode:  strings.TrimSpace(os.Getenv("DATABASE_SSL_MODE")),
		HTTPPort:         strings.TrimSpace(os.Getenv("HTTP_PORT")),

		WebSocketAllowedOrigins: splitList(os.Getenv("WEBSOCKET_ALLOWED_ORIGINS")),

		GCSProjectID: strings.TrimSpace(os.Getenv("GCS_PROJECT_ID")),
		GCSBucket:    strings.TrimSpace(os.Getenv("GCS_BUCKET")),
	}

	required := []struct {
		name  string
		value string
	}{
		{name: "ENVIRONMENT", value: envVars.Environment},
		{name: "DATABASE_HOST", value: envVars.DatabaseHost},
		{name: "DATABASE_PORT", value: envVars.DatabasePort},
		{name: "DATABASE_USERNAME", value: envVars.DatabaseUsername},
		{name: "DATABASE_PASSWORD", value: envVars.DatabasePassword},
		{name: "DATABASE_NAME", value: envVars.DatabaseName},
		{name: "DATABASE_SSL_MODE", value: envVars.DatabaseSSLMode},
		{name: "HTTP_PORT", value: envVars.HTTPPort},
	}
	for _, variable := range required {
		if variable.value == "" {
			return nil, fmt.Errorf("%s is required", variable.name)
		}
	}

	port, err := strconv.Atoi(envVars.DatabasePort)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("DATABASE_PORT must be a valid TCP port")
	}

	httpPort, err := strconv.Atoi(envVars.HTTPPort)
	if err != nil || httpPort < 1 || httpPort > 65535 {
		return nil, fmt.Errorf("HTTP_PORT must be a valid TCP port")
	}

	if envVars.SandboxLimits.MaxContexts, err = optionalPositiveInt("SANDBOX_MAX_CONTEXTS"); err != nil {
		return nil, err
	}
	if envVars.SandboxLimits.MaxConcurrentExecutions, err = optionalPositiveInt("SANDBOX_MAX_CONCURRENT_EXECUTIONS"); err != nil {
		return nil, err
	}

	return envVars, nil
}

// optionalPositiveInt reads name as a positive integer, or 0 when unset.
func optionalPositiveInt(name string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

// splitList parses a comma-separated list, dropping empty entries.
func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func openPostgres(envVars *envVariables) (*gorm.DB, error) {
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(envVars.DatabaseUsername, envVars.DatabasePassword),
		Host:   net.JoinHostPort(envVars.DatabaseHost, envVars.DatabasePort),
		Path:   envVars.DatabaseName,
		RawQuery: url.Values{
			"sslmode": []string{envVars.DatabaseSSLMode},
		}.Encode(),
	}).String()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
