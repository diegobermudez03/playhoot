package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api"
	"github.com/diegobermudez03/playhoot/game/usecases/checkvisibility"
	"github.com/diegobermudez03/playhoot/orchestrator"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle"
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
	ExecutorAddr     string
	GCSProjectID     string
	GCSBucket        string
	GCSSignedURLTTL  time.Duration
}

func main() {
	envVars, err := readEnvVariables()
	if err != nil {
		log.Fatalf("reading environment variables: %v", err)
	}

	db, err := openPostgres(envVars)
	if err != nil {
		log.Fatalf("opening PostgreSQL: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("getting PostgreSQL connection: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("closing PostgreSQL connection: %v", err)
		}
	}()

	if err := PostgresMigrate(db); err != nil {
		log.Fatalf("running PostgreSQL migrations: %v", err)
	}

	manager, closeInfrastructure, err := sessionlifecycle.NewProduction(context.Background(), db, sessionlifecycle.ProductionConfig{
		ExecutorAddr: envVars.ExecutorAddr,
		GCSProjectID: envVars.GCSProjectID,
		GCSBucket:    envVars.GCSBucket,
		SignedURLTTL: envVars.GCSSignedURLTTL,
	})
	if err != nil {
		log.Fatalf("connecting to the JavaScript Executor and object storage: %v", err)
	}
	defer func() {
		if err := closeInfrastructure(); err != nil {
			log.Printf("closing Executor and object storage connections: %v", err)
		}
	}()

	orch := orchestrator.New(checkvisibility.New(db), manager)

	server := api.NewServer(orch, manager)

	log.Printf("listening on :%s", envVars.HTTPPort)
	if err := http.ListenAndServe(":"+envVars.HTTPPort, server.Routes()); err != nil {
		log.Fatalf("serving HTTP: %v", err)
	}
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
		ExecutorAddr:     strings.TrimSpace(os.Getenv("EXECUTOR_ADDR")),
		GCSProjectID:     strings.TrimSpace(os.Getenv("GCS_PROJECT_ID")),
		GCSBucket:        strings.TrimSpace(os.Getenv("GCS_BUCKET")),
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
		{name: "EXECUTOR_ADDR", value: envVars.ExecutorAddr},
		{name: "GCS_PROJECT_ID", value: envVars.GCSProjectID},
		{name: "GCS_BUCKET", value: envVars.GCSBucket},
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

	// GCS_SIGNED_URL_TTL is optional (a Go duration such as "2m"); unset
	// keeps the default.
	if rawTTL := strings.TrimSpace(os.Getenv("GCS_SIGNED_URL_TTL")); rawTTL != "" {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("GCS_SIGNED_URL_TTL must be a positive duration such as 2m")
		}
		envVars.GCSSignedURLTTL = ttl
	}

	return envVars, nil
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
