//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/identity/kratos"
	identitypostgres "github.com/lupppig/credar/backend/internal/identity/postgres"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
	"github.com/lupppig/credar/backend/internal/tenancy"
	tenancypostgres "github.com/lupppig/credar/backend/internal/tenancy/postgres"
)

func TestAuthenticationAgainstKratos(t *testing.T) {
	publicURL := requiredEnvironment(t, "CREDAR_TEST_KRATOS_PUBLIC_URL")
	adminURL := requiredEnvironment(t, "CREDAR_TEST_KRATOS_ADMIN_URL")
	databaseURL := requiredEnvironment(t, "CREDAR_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	databaseConnection, err := database.New(database.Config{
		ConnectionString: databaseURL,
		MaxConnections:   4,
		MinConnections:   0,
	})
	if err != nil {
		t.Fatalf("construct database: %v", err)
	}
	if err := databaseConnection.Start(ctx); err != nil {
		t.Fatalf("start database: %v", err)
	}
	t.Cleanup(func() {
		if err := databaseConnection.Close(context.Background()); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	kratosClient, err := kratos.New(kratos.Config{
		PublicURL: publicURL,
		AdminURL:  adminURL,
		Timeout:   3 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct Kratos client: %v", err)
	}
	if err := kratosClient.Check(ctx); err != nil {
		t.Fatalf("check Kratos: %v", err)
	}

	repository := identitypostgres.New(databaseConnection, outbox.NewRepository(databaseConnection))
	tenancyRepository := tenancypostgres.New(databaseConnection, outbox.NewRepository(databaseConnection))
	handler := New(identity.NewAuthenticator(kratosClient, repository), tenancyRepository)
	router := gin.New()
	handler.RegisterRoutes(router)

	missing := performIntegrationRequest(t, router, "")
	assertProblemCode(t, missing, http.StatusUnauthorized, "authentication_required")

	email := "auth-" + uuid.NewString() + "@example.com"
	password := "integration-password-123!"
	kratosIdentityID := createKratosIdentity(t, ctx, adminURL, email, password)
	sessionToken := createKratosSession(t, ctx, publicURL, email, password)

	valid := performIntegrationRequest(t, router, sessionToken)
	if valid.Code != http.StatusOK {
		t.Fatalf("expected valid session to return 200, got %d: %s", valid.Code, valid.Body.String())
	}
	var validSession struct {
		User identity.User `json:"user"`
	}
	if err := json.Unmarshal(valid.Body.Bytes(), &validSession); err != nil {
		t.Fatalf("decode valid session: %v", err)
	}
	writeContext := requestcontext.WithIdentifiers(ctx, mustIntegrationUUIDv7(t), mustIntegrationUUIDv7(t))
	writeContext = requestcontext.WithUserID(writeContext, validSession.User.ID)
	createdTenant, err := tenancyRepository.CreateTenant(writeContext, tenancy.CreateTenantCommand{
		Name:           "Authentication Test Tenant",
		OwnerUserID:    validSession.User.ID,
		IdempotencyKey: "authentication-test-tenant",
	})
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	selected := performIntegrationRequestWithTenant(t, router, sessionToken, createdTenant.Tenant.ID)
	if selected.Code != http.StatusOK {
		t.Fatalf("expected selected tenant context to return 200, got %d: %s", selected.Code, selected.Body.String())
	}
	var selectedSession struct {
		Memberships    []identity.MembershipSummary `json:"memberships"`
		ActiveTenantID *string                      `json:"active_tenant_id"`
	}
	if err := json.Unmarshal(selected.Body.Bytes(), &selectedSession); err != nil {
		t.Fatalf("decode selected session: %v", err)
	}
	if len(selectedSession.Memberships) != 1 ||
		selectedSession.ActiveTenantID == nil ||
		*selectedSession.ActiveTenantID != createdTenant.Tenant.ID {
		t.Fatalf("unexpected selected session context: %+v", selectedSession)
	}
	malformedTenant := performIntegrationRequestWithTenant(t, router, sessionToken, "not-a-uuid")
	assertProblemCode(t, malformedTenant, http.StatusBadRequest, "invalid_tenant_context")
	inactiveTenant := performIntegrationRequestWithTenant(t, router, sessionToken, mustIntegrationUUIDv7(t))
	assertProblemCode(t, inactiveTenant, http.StatusForbidden, "membership_inactive")

	invalid := performIntegrationRequest(t, router, "not-a-session")
	assertProblemCode(t, invalid, http.StatusUnauthorized, "invalid_session")

	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get database pool: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE identity.users SET kratos_identity_id = $1 WHERE kratos_identity_id = $2`,
		uuid.NewString(),
		kratosIdentityID,
	); err != nil {
		t.Fatalf("create mapping conflict: %v", err)
	}
	conflict := performIntegrationRequest(t, router, sessionToken)
	assertProblemCode(t, conflict, http.StatusConflict, "identity_mapping_conflict")

	deadline := time.Now().Add(12 * time.Second)
	for {
		expired := performIntegrationRequest(t, router, sessionToken)
		if expired.Code == http.StatusUnauthorized {
			assertProblemCode(t, expired, http.StatusUnauthorized, "invalid_session")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Kratos session did not expire before deadline; last response was %d: %s", expired.Code, expired.Body.String())
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func createKratosIdentity(
	t *testing.T,
	ctx context.Context,
	adminURL string,
	email string,
	password string,
) string {
	t.Helper()
	payload := map[string]any{
		"schema_id": "default",
		"traits":    map[string]string{"email": email},
		"credentials": map[string]any{
			"password": map[string]any{
				"config": map[string]string{"password": password},
			},
		},
	}
	var responseBody struct {
		ID string `json:"id"`
	}
	doJSON(t, ctx, http.MethodPost, adminURL+"/admin/identities", payload, http.StatusCreated, &responseBody)
	if responseBody.ID == "" {
		t.Fatal("Kratos create identity response omitted the identity ID")
	}
	return responseBody.ID
}

func createKratosSession(
	t *testing.T,
	ctx context.Context,
	publicURL string,
	email string,
	password string,
) string {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL+"/self-service/login/api", nil)
	if err != nil {
		t.Fatalf("create login flow request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("create login flow: %v", err)
	}
	defer response.Body.Close()
	var flow struct {
		ID string `json:"id"`
	}
	decodeExpectedJSON(t, response, http.StatusOK, &flow)

	payload := map[string]string{
		"method":     "password",
		"identifier": email,
		"password":   password,
	}
	var login struct {
		SessionToken string `json:"session_token"`
	}
	doJSON(
		t,
		ctx,
		http.MethodPost,
		publicURL+"/self-service/login?flow="+flow.ID,
		payload,
		http.StatusOK,
		&login,
	)
	if login.SessionToken == "" {
		t.Fatal("Kratos login response omitted the session token")
	}
	return login.SessionToken
}

func doJSON(
	t *testing.T,
	ctx context.Context,
	method string,
	url string,
	payload any,
	expectedStatus int,
	target any,
) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer response.Body.Close()
	decodeExpectedJSON(t, response, expectedStatus, target)
}

func decodeExpectedJSON(t *testing.T, response *http.Response, expectedStatus int, target any) {
	t.Helper()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, response.StatusCode, string(body))
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func performIntegrationRequest(
	t *testing.T,
	router http.Handler,
	sessionToken string,
) *httptest.ResponseRecorder {
	return performIntegrationRequestWithTenant(t, router, sessionToken, "")
}

func performIntegrationRequestWithTenant(
	t *testing.T,
	router http.Handler,
	sessionToken string,
	tenantID string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v1/session/context", nil)
	requestID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create request ID: %v", err)
	}
	request = request.WithContext(requestcontext.WithIdentifiers(request.Context(), requestID, requestID))
	if sessionToken != "" {
		request.Header.Set("X-Session-Token", sessionToken)
	}
	if tenantID != "" {
		request.Header.Set("X-Tenant-ID", tenantID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func mustIntegrationUUIDv7(t *testing.T) string {
	t.Helper()
	value, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create UUIDv7: %v", err)
	}
	return value
}

func assertProblemCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body.Code != code {
		t.Fatalf("expected problem code %q, got %q", code, body.Code)
	}
}

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skip(fmt.Sprintf("%s is required for Kratos integration tests", name))
	}
	return value
}
