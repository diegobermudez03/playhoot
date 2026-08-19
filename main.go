package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

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
