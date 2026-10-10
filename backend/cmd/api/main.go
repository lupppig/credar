package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lupppig/credar/backend/internal/identity"
	identityhttp "github.com/lupppig/credar/backend/internal/identity/httpapi"
	"github.com/lupppig/credar/backend/internal/identity/kratos"
	identitypostgres "github.com/lupppig/credar/backend/internal/identity/postgres"
	"github.com/lupppig/credar/backend/internal/platform/appruntime"
	"github.com/lupppig/credar/backend/internal/platform/config"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/dependency"
	"github.com/lupppig/credar/backend/internal/platform/eventconsumer"
	"github.com/lupppig/credar/backend/internal/platform/healthcheck"
	platformhttp "github.com/lupppig/credar/backend/internal/platform/httpserver"
	"github.com/lupppig/credar/backend/internal/platform/logging"
	"github.com/lupppig/credar/backend/internal/platform/natsclient"
	"github.com/lupppig/credar/backend/internal/platform/objectstorage"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/redisclient"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	configuration, err := config.Load(os.LookupEnv)
	if err != nil {
		bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		bootstrapLogger.Error("invalid runtime configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(os.Stdout, configuration.LogLevel, configuration.Environment)
	databaseConnection, err := database.New(database.Config{
		ConnectionString: configuration.Database.URL.Reveal(),
		MaxConnections:   configuration.Database.MaxConnections,
		MinConnections:   configuration.Database.MinConnections,
	})
	if err != nil {
		logger.Error("configure PostgreSQL", "error", err)
		os.Exit(1)
	}
	natsConnection, err := natsclient.New(configuration.NATS.URL.Reveal())
	if err != nil {
		logger.Error("configure NATS", "error", err)
		os.Exit(1)
	}
	redisConnection, err := redisclient.New(redisclient.Config{
		URL:       configuration.Redis.URL.Reveal(),
		KeyPrefix: configuration.Redis.KeyPrefix,
	})
	if err != nil {
		logger.Error("configure Redis", "error", err)
		os.Exit(1)
	}
	objectStore, err := objectstorage.New(objectstorage.Config{
		Endpoint:  configuration.ObjectStorage.Endpoint,
		Bucket:    configuration.ObjectStorage.Bucket,
		AccessKey: configuration.ObjectStorage.AccessKey.Reveal(),
		SecretKey: configuration.ObjectStorage.SecretKey.Reveal(),
	})
	if err != nil {
		logger.Error("configure object storage", "error", err)
		os.Exit(1)
	}
	outboxRepository := outbox.NewRepository(databaseConnection)
	kratosClient, err := kratos.New(kratos.Config{
		PublicURL: configuration.Kratos.PublicURL,
		AdminURL:  configuration.Kratos.AdminURL,
		Timeout:   configuration.Kratos.Timeout,
	})
	if err != nil {
		logger.Error("configure Kratos", "error", err)
		os.Exit(1)
	}
	identityRepository := identitypostgres.New(databaseConnection, outboxRepository)
	authenticator := identity.NewAuthenticator(kratosClient, identityRepository)
	identityHandler := identityhttp.New(authenticator)
	outboxRelay, err := outbox.NewRelay(outboxRepository, natsConnection, outbox.RelayConfig{
		PollInterval:  configuration.Outbox.PollInterval,
		LeaseDuration: configuration.Outbox.LeaseDuration,
		BatchSize:     configuration.Outbox.BatchSize,
		MaxAttempts:   configuration.Outbox.MaxAttempts,
		BaseBackoff:   configuration.Outbox.BaseBackoff,
		MaxBackoff:    configuration.Outbox.MaxBackoff,
	}, logger)
	if err != nil {
		logger.Error("configure outbox relay", "error", err)
		os.Exit(1)
	}
	proofConsumer, err := eventconsumer.New(databaseConnection, natsConnection, logger)
	if err != nil {
		logger.Error("configure infrastructure event consumer", "error", err)
		os.Exit(1)
	}
	dependencies, err := dependency.NewSet(
		configuration.ReadinessTimeout,
		databaseConnection,
		natsConnection,
		redisConnection,
		objectStore,
		kratosClient,
		proofConsumer,
		outboxRelay,
	)
	if err != nil {
		logger.Error("configure runtime dependencies", "error", err)
		os.Exit(1)
	}
	server := platformhttp.New(configuration.HTTPAddress, logger, dependencies, identityHandler)

	shutdownSignals, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	if err := appruntime.Run(
		shutdownSignals,
		server,
		dependencies,
		configuration.ShutdownTimeout,
		logger,
	); err != nil {
		logger.Error("API process stopped with an error", "error", err)
		os.Exit(1)
	}
}

func runHealthcheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return healthcheck.Check(
		ctx,
		&http.Client{Timeout: 2 * time.Second},
		"http://127.0.0.1:8080/readyz",
	)
}
