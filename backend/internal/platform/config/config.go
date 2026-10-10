// Package config loads and validates process configuration at startup.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvironment             = "local"
	defaultLogLevel                = "info"
	defaultHTTPAddress             = ":8080"
	defaultShutdownTimeoutSeconds  = 10
	defaultReadinessTimeoutSeconds = 2
	defaultDatabaseURL             = "postgres://credar:credar_local@127.0.0.1:5432/credar?sslmode=disable"
	defaultDatabaseMaxConnections  = 10
	defaultDatabaseMinConnections  = 1
	defaultNATSURL                 = "nats://127.0.0.1:4222"
	defaultOutboxPollMilliseconds  = 250
	defaultOutboxLeaseSeconds      = 30
	defaultOutboxBatchSize         = 100
	defaultOutboxMaxAttempts       = 8
	defaultOutboxBaseBackoff       = 1
	defaultOutboxMaxBackoff        = 300
	defaultRedisURL                = "redis://127.0.0.1:6379/0"
	defaultRedisKeyPrefix          = "credar"
	defaultObjectStorageEndpoint   = "http://127.0.0.1:8333"
	defaultObjectStorageBucket     = "credar-local"
	defaultObjectStorageAccessKey  = "local-access-key"
	defaultObjectStorageSecretKey  = "local-secret-key"
	defaultKratosPublicURL         = "http://127.0.0.1:4433"
	defaultKratosAdminURL          = "http://127.0.0.1:4434"
	defaultKratosTimeoutSeconds    = 2
)

var (
	redisKeyPrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,62}$`)
	bucketNamePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
)

// LookupEnv matches os.LookupEnv and makes configuration loading deterministic
// in tests.
type LookupEnv func(string) (string, bool)

// Config contains the validated values consumed by the API process.
type Config struct {
	Environment      string
	LogLevel         slog.Level
	HTTPAddress      string
	ShutdownTimeout  time.Duration
	ReadinessTimeout time.Duration
	Database         DatabaseConfig
	NATS             NATSConfig
	Outbox           OutboxConfig
	Redis            RedisConfig
	ObjectStorage    ObjectStorageConfig
	Kratos           KratosConfig
}

// NATSConfig contains the sensitive JetStream connection endpoint.
type NATSConfig struct {
	URL Secret
}

// OutboxConfig bounds asynchronous relay behavior.
type OutboxConfig struct {
	PollInterval  time.Duration
	LeaseDuration time.Duration
	BatchSize     int
	MaxAttempts   int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
}

// RedisConfig identifies the isolated ephemeral keyspace.
type RedisConfig struct {
	URL       Secret
	KeyPrefix string
}

// ObjectStorageConfig contains the local S3-compatible adapter settings.
type ObjectStorageConfig struct {
	Endpoint  string
	Bucket    string
	AccessKey Secret
	SecretKey Secret
}

// KratosConfig identifies the server-side identity endpoints and bounds every
// authentication request.
type KratosConfig struct {
	PublicURL string
	AdminURL  string
	Timeout   time.Duration
}

// DatabaseConfig contains the PostgreSQL connection and pool policy.
type DatabaseConfig struct {
	URL            Secret
	MaxConnections int32
	MinConnections int32
}

// Secret prevents credentials from being exposed by ordinary string and Go
// formatting. Reveal is reserved for the adapter that consumes the value.
type Secret struct {
	value string
}

// Reveal returns the sensitive configuration value to its owning adapter.
func (secret Secret) Reveal() string {
	return secret.value
}

func (Secret) String() string {
	return "<redacted>"
}

func (Secret) GoString() string {
	return "<redacted>"
}

// Load reads every runtime value once and rejects malformed configuration.
func Load(lookup LookupEnv) (Config, error) {
	environment := valueOrDefault(lookup, "CREDAR_ENVIRONMENT", defaultEnvironment)
	switch environment {
	case "local", "test", "sandbox":
	default:
		return Config{}, fmt.Errorf(
			"CREDAR_ENVIRONMENT must be one of local, test, or sandbox, got %q",
			environment,
		)
	}

	logLevelValue := valueOrDefault(lookup, "CREDAR_LOG_LEVEL", defaultLogLevel)
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(logLevelValue)); err != nil {
		return Config{}, fmt.Errorf("CREDAR_LOG_LEVEL %q is invalid: %w", logLevelValue, err)
	}

	httpAddress := valueOrDefault(lookup, "CREDAR_HTTP_ADDRESS", defaultHTTPAddress)
	if err := validateAddress(httpAddress); err != nil {
		return Config{}, fmt.Errorf("CREDAR_HTTP_ADDRESS %q is invalid: %w", httpAddress, err)
	}

	shutdownTimeout, err := seconds(
		lookup,
		"CREDAR_HTTP_SHUTDOWN_TIMEOUT_SECONDS",
		defaultShutdownTimeoutSeconds,
	)
	if err != nil {
		return Config{}, err
	}

	readinessTimeout, err := seconds(
		lookup,
		"CREDAR_READINESS_TIMEOUT_SECONDS",
		defaultReadinessTimeoutSeconds,
	)
	if err != nil {
		return Config{}, err
	}

	databaseURL := valueOrDefault(lookup, "CREDAR_DATABASE_URL", defaultDatabaseURL)
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, fmt.Errorf("CREDAR_DATABASE_URL is invalid: %w", err)
	}
	databaseMaxConnections, err := integer(
		lookup,
		"CREDAR_DATABASE_MAX_CONNECTIONS",
		defaultDatabaseMaxConnections,
		1,
		100,
	)
	if err != nil {
		return Config{}, err
	}
	databaseMinConnections, err := integer(
		lookup,
		"CREDAR_DATABASE_MIN_CONNECTIONS",
		defaultDatabaseMinConnections,
		0,
		databaseMaxConnections,
	)
	if err != nil {
		return Config{}, err
	}

	natsURL := valueOrDefault(lookup, "CREDAR_NATS_URL", defaultNATSURL)
	if err := validateNATSURL(natsURL); err != nil {
		return Config{}, fmt.Errorf("CREDAR_NATS_URL is invalid: %w", err)
	}
	outboxPollMilliseconds, err := integer(
		lookup,
		"CREDAR_OUTBOX_POLL_MILLISECONDS",
		defaultOutboxPollMilliseconds,
		10,
		60000,
	)
	if err != nil {
		return Config{}, err
	}
	outboxLeaseSeconds, err := integer(
		lookup,
		"CREDAR_OUTBOX_LEASE_SECONDS",
		defaultOutboxLeaseSeconds,
		1,
		3600,
	)
	if err != nil {
		return Config{}, err
	}
	outboxBatchSize, err := integer(
		lookup,
		"CREDAR_OUTBOX_BATCH_SIZE",
		defaultOutboxBatchSize,
		1,
		1000,
	)
	if err != nil {
		return Config{}, err
	}
	outboxMaxAttempts, err := integer(
		lookup,
		"CREDAR_OUTBOX_MAX_ATTEMPTS",
		defaultOutboxMaxAttempts,
		1,
		100,
	)
	if err != nil {
		return Config{}, err
	}
	outboxBaseBackoff, err := integer(
		lookup,
		"CREDAR_OUTBOX_BASE_BACKOFF_SECONDS",
		defaultOutboxBaseBackoff,
		1,
		3600,
	)
	if err != nil {
		return Config{}, err
	}
	outboxMaxBackoff, err := integer(
		lookup,
		"CREDAR_OUTBOX_MAX_BACKOFF_SECONDS",
		defaultOutboxMaxBackoff,
		outboxBaseBackoff,
		86400,
	)
	if err != nil {
		return Config{}, err
	}

	redisURL := valueOrDefault(lookup, "CREDAR_REDIS_URL", defaultRedisURL)
	if err := validateRedisURL(redisURL); err != nil {
		return Config{}, fmt.Errorf("CREDAR_REDIS_URL is invalid: %w", err)
	}
	redisKeyPrefix := valueOrDefault(lookup, "CREDAR_REDIS_KEY_PREFIX", defaultRedisKeyPrefix)
	if !redisKeyPrefixPattern.MatchString(redisKeyPrefix) {
		return Config{}, fmt.Errorf("CREDAR_REDIS_KEY_PREFIX must be 2-63 lowercase letters, digits, underscores, or hyphens")
	}

	objectStorageEndpoint := valueOrDefault(
		lookup,
		"CREDAR_OBJECT_STORAGE_ENDPOINT",
		defaultObjectStorageEndpoint,
	)
	if err := validateObjectStorageEndpoint(objectStorageEndpoint); err != nil {
		return Config{}, fmt.Errorf("CREDAR_OBJECT_STORAGE_ENDPOINT is invalid: %w", err)
	}
	objectStorageBucket := valueOrDefault(
		lookup,
		"CREDAR_OBJECT_STORAGE_BUCKET",
		defaultObjectStorageBucket,
	)
	if !validBucketName(objectStorageBucket) {
		return Config{}, fmt.Errorf("CREDAR_OBJECT_STORAGE_BUCKET must be a valid 3-63 character lowercase S3 bucket name")
	}
	objectStorageAccessKey := valueOrDefault(
		lookup,
		"CREDAR_OBJECT_STORAGE_ACCESS_KEY",
		defaultObjectStorageAccessKey,
	)
	if objectStorageAccessKey == "" {
		return Config{}, fmt.Errorf("CREDAR_OBJECT_STORAGE_ACCESS_KEY must not be empty")
	}
	objectStorageSecretKey := valueOrDefault(
		lookup,
		"CREDAR_OBJECT_STORAGE_SECRET_KEY",
		defaultObjectStorageSecretKey,
	)
	if objectStorageSecretKey == "" {
		return Config{}, fmt.Errorf("CREDAR_OBJECT_STORAGE_SECRET_KEY must not be empty")
	}

	kratosPublicURL := valueOrDefault(lookup, "CREDAR_KRATOS_PUBLIC_URL", defaultKratosPublicURL)
	if err := validateHTTPServiceURL(kratosPublicURL); err != nil {
		return Config{}, fmt.Errorf("CREDAR_KRATOS_PUBLIC_URL is invalid: %w", err)
	}
	kratosAdminURL := valueOrDefault(lookup, "CREDAR_KRATOS_ADMIN_URL", defaultKratosAdminURL)
	if err := validateHTTPServiceURL(kratosAdminURL); err != nil {
		return Config{}, fmt.Errorf("CREDAR_KRATOS_ADMIN_URL is invalid: %w", err)
	}
	kratosTimeout, err := seconds(
		lookup,
		"CREDAR_KRATOS_REQUEST_TIMEOUT_SECONDS",
		defaultKratosTimeoutSeconds,
	)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:      environment,
		LogLevel:         logLevel,
		HTTPAddress:      httpAddress,
		ShutdownTimeout:  shutdownTimeout,
		ReadinessTimeout: readinessTimeout,
		Database: DatabaseConfig{
			URL:            Secret{value: databaseURL},
			MaxConnections: int32(databaseMaxConnections),
			MinConnections: int32(databaseMinConnections),
		},
		NATS: NATSConfig{URL: Secret{value: natsURL}},
		Outbox: OutboxConfig{
			PollInterval:  time.Duration(outboxPollMilliseconds) * time.Millisecond,
			LeaseDuration: time.Duration(outboxLeaseSeconds) * time.Second,
			BatchSize:     outboxBatchSize,
			MaxAttempts:   outboxMaxAttempts,
			BaseBackoff:   time.Duration(outboxBaseBackoff) * time.Second,
			MaxBackoff:    time.Duration(outboxMaxBackoff) * time.Second,
		},
		Redis: RedisConfig{
			URL:       Secret{value: redisURL},
			KeyPrefix: redisKeyPrefix,
		},
		ObjectStorage: ObjectStorageConfig{
			Endpoint:  objectStorageEndpoint,
			Bucket:    objectStorageBucket,
			AccessKey: Secret{value: objectStorageAccessKey},
			SecretKey: Secret{value: objectStorageSecretKey},
		},
		Kratos: KratosConfig{
			PublicURL: kratosPublicURL,
			AdminURL:  kratosAdminURL,
			Timeout:   kratosTimeout,
		},
	}, nil
}

func valueOrDefault(lookup LookupEnv, name string, fallback string) string {
	value, ok := lookup(name)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func seconds(lookup LookupEnv, name string, fallback int) (time.Duration, error) {
	parsed, err := integer(lookup, name, fallback, 1, 60)
	if err != nil {
		return 0, err
	}
	return time.Duration(parsed) * time.Second, nil
}

func integer(lookup LookupEnv, name string, fallback int, minimum int, maximum int) (int, error) {
	value := strconv.Itoa(fallback)
	if configured, ok := lookup(name); ok {
		value = strings.TrimSpace(configured)
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf(
			"%s must be an integer from %d to %d, got %q",
			name,
			minimum,
			maximum,
			value,
		)
	}
	return parsed, nil
}

func validateAddress(address string) error {
	if strings.TrimSpace(address) != address || address == "" {
		return fmt.Errorf("must be a non-empty host:port value without surrounding whitespace")
	}

	_, portValue, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("must be a host:port value: %w", err)
	}

	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be an integer from 1 to 65535")
	}
	return nil
}

func validateDatabaseURL(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("must be a PostgreSQL URL")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("scheme must be postgres or postgresql")
	}
	if parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" {
		return fmt.Errorf("host and database name are required")
	}
	return nil
}

func validateNATSURL(natsURL string) error {
	parsed, err := url.Parse(natsURL)
	if err != nil {
		return fmt.Errorf("must be a NATS URL")
	}
	if parsed.Scheme != "nats" && parsed.Scheme != "tls" {
		return fmt.Errorf("scheme must be nats or tls")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func validateRedisURL(redisURL string) error {
	parsed, err := url.Parse(redisURL)
	if err != nil {
		return fmt.Errorf("must be a Redis URL")
	}
	if parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return fmt.Errorf("scheme must be redis or rediss")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func validateObjectStorageEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("must be an HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("credentials, paths, queries, and fragments are not allowed")
	}
	return nil
}

func validateHTTPServiceURL(serviceURL string) error {
	parsed, err := url.Parse(serviceURL)
	if err != nil {
		return fmt.Errorf("must be an HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("credentials, queries, and fragments are not allowed")
	}
	return nil
}

func validBucketName(name string) bool {
	return len(name) >= 3 &&
		len(name) <= 63 &&
		bucketNamePattern.MatchString(name) &&
		!strings.Contains(name, "..") &&
		!strings.Contains(name, ".-") &&
		!strings.Contains(name, "-.") &&
		net.ParseIP(name) == nil
}
