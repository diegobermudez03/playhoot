package main

import (
	"context"
	"crypto/rand"
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
	"github.com/diegobermudez03/playhoot/identity/usecases/endsession"
	"github.com/diegobermudez03/playhoot/identity/usecases/listoperations"
	"github.com/diegobermudez03/playhoot/identity/usecases/refreshcredentials"
	"github.com/diegobermudez03/playhoot/orchestrator/playlifecycle"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/usecases/ownership"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/liveconnection"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/replayrecording"
	"github.com/diegobermudez03/playhoot/play/session/batches/reconcilesessions"
	sessionmigration "github.com/diegobermudez03/playhoot/play/session/migration"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	"github.com/diegobermudez03/playhoot/play/session/usecases/checkadminjoin"
	"github.com/diegobermudez03/playhoot/play/session/usecases/checklivesession"
	"github.com/diegobermudez03/playhoot/play/session/usecases/checkplayerjoin"
	"github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess"
	"github.com/diegobermudez03/playhoot/play/session/usecases/loadruntimemanifest"
	"github.com/diegobermudez03/playhoot/play/session/usecases/loadsessionlimits"
	"github.com/diegobermudez03/playhoot/play/session/usecases/recordgamefault"
	"github.com/diegobermudez03/playhoot/play/session/usecases/recordlimithits"
	"github.com/diegobermudez03/playhoot/play/session/usecases/resolvejoincode"
	"github.com/diegobermudez03/playhoot/play/session/usecases/storereplaychunk"
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

	// CORSAllowedOrigins are the browser origins whose pages may call the
	// REST routes, and WebSocketAllowedOrigins those that may open live
	// connections besides the server's own: comma-separated origins such as
	// https://play.example.com, or "*" for any. Optional: unset allows none.
	CORSAllowedOrigins      []string
	WebSocketAllowedOrigins []string

	// TrustedProxyHops is how many proxies in front of the server append the
	// client address to X-Forwarded-For (1 behind Cloud Run alone). Optional:
	// unset trusts none and limits by the connection's own address.
	TrustedProxyHops int

	// MaxLiveConnections bounds this instance's live connections and
	// MaxLiveSessions its live Sessions. Optional, zero takes the defaults.
	MaxLiveConnections int
	MaxLiveSessions    int

	// IdentityAccessTokenKey signs access tokens (at least 32 bytes). Required
	// in production; elsewhere a random key is generated when unset, so access
	// tokens do not survive a restart (refresh tokens do).
	IdentityAccessTokenKey []byte

	// IdentityDevTestUsers accepts the fixed test account tokens (test-user-1,
	// test-user-2) until accounts can sign in locally. Never in production.
	IdentityDevTestUsers bool

	// GCSProjectID and GCSBucket name the object storage of game version
	// content. Optional: without them every storage call fails. An emulator
	// is selected with STORAGE_EMULATOR_HOST.
	GCSProjectID string
	GCSBucket    string

	// Sandbox limits for this instance; optional, zero takes the defaults.
	SandboxLimits sandbox.HostLimits

	// Replay recording tuning; optional, zero takes the defaults.
	Replay replayrecording.Config
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
	owners := ownership.New(checklivesession.New(db))
	replayRecorder := replayrecording.New(storereplaychunk.New(db, store), envVars.Replay)
	// Closed after the controllers, deferred earlier: each stops its
	// recording first, then pending chunks get a few seconds to be stored.
	defer replayRecorder.Close()
	liveConnections := liveconnection.New(envVars.MaxLiveSessions, checkadminjoin.New(db), checkplayerjoin.New(db), loadruntimemanifest.New(db), loadsessionlimits.New(db), sessionLifecycle, sessionLifecycle, sessionLifecycle, sessionLifecycle, frontendaccess.New(db, store), sandboxHost, replayRecorder, owners, recordgamefault.New(db), recordlimithits.New(db))
	// Closed before the sandbox, deferred later: controllers stop first, so
	// shutting down never fails a running game.
	defer liveConnections.Close()
	authorizer, err := authorize.New(envVars.IdentityAccessTokenKey, envVars.IdentityDevTestUsers)
	if err != nil {
		return fmt.Errorf("creating authorizer: %w", err)
	}
	guests, err := createguest.New(db, envVars.IdentityAccessTokenKey)
	if err != nil {
		return fmt.Errorf("creating guest creation: %w", err)
	}
	refreshes, err := refreshcredentials.New(db, envVars.IdentityAccessTokenKey)
	if err != nil {
		return fmt.Errorf("creating credential refresh: %w", err)
	}
	server := api.NewServer(
		authorizer,
		guests,
		listoperations.New(),
		refreshes,
		endsession.New(db),
		playLifecycle,
		owners,
		liveConnections,
		liveConnections,
		resolvejoincode.New(db),
		api.Config{
			CORSAllowedOrigins:      envVars.CORSAllowedOrigins,
			WebSocketAllowedOrigins: envVars.WebSocketAllowedOrigins,
			TrustedProxyHops:        envVars.TrustedProxyHops,
			MaxLiveConnections:      envVars.MaxLiveConnections,
		},
	)

	// The timeouts bound a request's exchange; a live connection is no longer
	// subject to them once upgraded, since upgrading clears its deadlines.
	httpServer := &http.Server{
		Addr:              ":" + envVars.HTTPPort,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Every instance reconciles: ends the Sessions nothing live will end.
	reconcileCtx, stopReconciling := context.WithCancel(context.Background())
	reconciling := make(chan struct{})
	go func() {
		defer close(reconciling)
		reconcilesessions.New(db).Run(reconcileCtx)
	}()
	defer func() {
		stopReconciling()
		<-reconciling
	}()

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

	if envVars.CORSAllowedOrigins, err = allowedOrigins("CORS_ALLOWED_ORIGINS"); err != nil {
		return nil, err
	}
	if envVars.WebSocketAllowedOrigins, err = allowedOrigins("WEBSOCKET_ALLOWED_ORIGINS"); err != nil {
		return nil, err
	}
	if envVars.TrustedProxyHops, err = optionalPositiveInt("TRUSTED_PROXY_HOPS"); err != nil {
		return nil, err
	}
	if envVars.MaxLiveConnections, err = optionalPositiveInt("LIVE_MAX_CONNECTIONS"); err != nil {
		return nil, err
	}
	if envVars.MaxLiveSessions, err = optionalPositiveInt("RUNTIME_MAX_LIVE_SESSIONS"); err != nil {
		return nil, err
	}
	if envVars.SandboxLimits.MaxContexts, err = optionalPositiveInt("SANDBOX_MAX_CONTEXTS"); err != nil {
		return nil, err
	}
	if envVars.SandboxLimits.MaxConcurrentExecutions, err = optionalPositiveInt("SANDBOX_MAX_CONCURRENT_EXECUTIONS"); err != nil {
		return nil, err
	}
	if envVars.Replay.StateEveryTicks, err = optionalPositiveInt("REPLAY_STATE_SAMPLE_EVERY_N_TICKS"); err != nil {
		return nil, err
	}
	if envVars.Replay.ChunkDuration, err = optionalPositiveDuration("REPLAY_CHUNK_DURATION"); err != nil {
		return nil, err
	}
	if envVars.Replay.MaxPendingChunks, err = optionalPositiveInt("REPLAY_MAX_PENDING_CHUNKS"); err != nil {
		return nil, err
	}
	if err := readIdentityVariables(envVars); err != nil {
		return nil, err
	}

	return envVars, nil
}

// allowedOrigins reads name as a list of browser origins, or none when unset.
func allowedOrigins(name string) ([]string, error) {
	origins, err := api.ParseAllowedOrigins(os.Getenv(name))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return origins, nil
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

// optionalPositiveDuration reads name as a positive Go duration such as
// "60s", or 0 when unset.
func optionalPositiveDuration(name string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration such as 60s", name)
	}
	return value, nil
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

// readIdentityVariables reads Identity's key and its development bypass,
// refusing the bypass in production.
func readIdentityVariables(envVars *envVariables) error {
	production := strings.EqualFold(envVars.Environment, "production")
	key := os.Getenv("IDENTITY_ACCESS_TOKEN_KEY")
	switch {
	case key != "":
		envVars.IdentityAccessTokenKey = []byte(key)
	case production:
		return fmt.Errorf("IDENTITY_ACCESS_TOKEN_KEY is required in production")
	default:
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return fmt.Errorf("generating an access token key: %w", err)
		}
		envVars.IdentityAccessTokenKey = generated
		log.Printf("IDENTITY_ACCESS_TOKEN_KEY unset: using a random key, access tokens end with this process")
	}
	if len(envVars.IdentityAccessTokenKey) < 32 {
		return fmt.Errorf("IDENTITY_ACCESS_TOKEN_KEY must have at least 32 bytes")
	}

	devTestUsers := strings.TrimSpace(os.Getenv("IDENTITY_DEV_TEST_USERS"))
	if devTestUsers != "" {
		enabled, err := strconv.ParseBool(devTestUsers)
		if err != nil {
			return fmt.Errorf("IDENTITY_DEV_TEST_USERS must be true or false")
		}
		if enabled && production {
			return fmt.Errorf("IDENTITY_DEV_TEST_USERS cannot be enabled in production")
		}
		envVars.IdentityDevTestUsers = enabled
	}
	return nil
}
