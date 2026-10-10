package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	authorizationpostgres "github.com/lupppig/credar/backend/internal/authorization/postgres"
	"github.com/lupppig/credar/backend/internal/identity/kratos"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

func main() {
	identityID := flag.String("kratos-identity-id", "", "Kratos identity UUID to promote exactly once")
	flag.Parse()
	if *identityID == "" {
		fail("--kratos-identity-id is required")
	}
	databaseURL := os.Getenv("CREDAR_DATABASE_URL")
	adminURL := os.Getenv("CREDAR_KRATOS_ADMIN_URL")
	if databaseURL == "" || adminURL == "" {
		fail("CREDAR_DATABASE_URL and CREDAR_KRATOS_ADMIN_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.New(database.Config{ConnectionString: databaseURL, MaxConnections: 2})
	if err != nil {
		fail(err.Error())
	}
	if err := db.Start(ctx); err != nil {
		fail(err.Error())
	}
	defer func() { _ = db.Close(context.Background()) }()
	client, err := kratos.New(kratos.Config{PublicURL: adminURL, AdminURL: adminURL, Timeout: 10 * time.Second})
	if err != nil {
		fail(err.Error())
	}
	external, err := client.LookupIdentity(ctx, *identityID)
	if err != nil {
		fail(err.Error())
	}
	requestID, err := identifier.NewUUIDv7()
	if err != nil {
		fail(err.Error())
	}
	correlationID, err := identifier.NewUUIDv7()
	if err != nil {
		fail(err.Error())
	}
	ctx = requestcontext.WithIdentifiers(ctx, requestID, correlationID)
	repository := authorizationpostgres.New(db, outbox.NewRepository(db))
	user, err := repository.BootstrapPlatformAdmin(ctx, external)
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("bootstrapped platform administrator %s (%s)\n", user.ID, user.Email)
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
