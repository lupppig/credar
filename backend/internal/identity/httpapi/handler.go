package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/httpproblem"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const authenticatedUserKey = "identity.authenticated_user"

type Authenticator interface {
	Authenticate(context.Context, identity.Credentials) (identity.User, error)
}

type Handler struct {
	authenticator Authenticator
	sessions      identity.SessionContextResolver
}

func New(
	authenticator Authenticator,
	sessionResolvers ...identity.SessionContextResolver,
) *Handler {
	handler := &Handler{authenticator: authenticator}
	if len(sessionResolvers) > 0 {
		handler.sessions = sessionResolvers[0]
	}
	return handler
}

func (handler *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/v1/session/context", handler.requireSession(), handler.sessionContext)
}

func (handler *Handler) requireSession() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		user, err := handler.authenticator.Authenticate(
			ginContext.Request.Context(),
			identity.Credentials{
				Cookie:       ginContext.GetHeader("Cookie"),
				SessionToken: ginContext.GetHeader("X-Session-Token"),
			},
		)
		if err != nil {
			writeAuthenticationProblem(ginContext, err)
			return
		}

		ginContext.Set(authenticatedUserKey, user)
		requestContext := requestcontext.WithUserID(ginContext.Request.Context(), user.ID)
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		ginContext.Next()
	}
}

func (handler *Handler) sessionContext(ginContext *gin.Context) {
	value, exists := ginContext.Get(authenticatedUserKey)
	user, valid := value.(identity.User)
	if !exists || !valid {
		httpproblem.InternalServerError(ginContext)
		return
	}

	memberships := make([]identity.MembershipSummary, 0)
	var activeTenantID *string
	if handler.sessions != nil {
		var err error
		memberships, activeTenantID, err = handler.sessions.ResolveSessionContext(
			ginContext.Request.Context(),
			user.ID,
			ginContext.GetHeader("X-Tenant-ID"),
		)
		if err != nil {
			writeSessionContextProblem(ginContext, err)
			return
		}
	}
	if activeTenantID != nil {
		requestContext := requestcontext.WithTenantID(ginContext.Request.Context(), *activeTenantID)
		ginContext.Request = ginContext.Request.WithContext(requestContext)
	}

	ginContext.JSON(http.StatusOK, struct {
		User           identity.User                `json:"user"`
		Memberships    []identity.MembershipSummary `json:"memberships"`
		ActiveTenantID *string                      `json:"active_tenant_id"`
	}{
		User:           user,
		Memberships:    memberships,
		ActiveTenantID: activeTenantID,
	})
}

func writeSessionContextProblem(ginContext *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidTenantContext):
		httpproblem.Write(
			ginContext,
			http.StatusBadRequest,
			"invalid_tenant_context",
			"Invalid tenant context",
			"X-Tenant-ID must be a canonical UUIDv7.",
		)
	case errors.Is(err, identity.ErrMembershipInactive):
		httpproblem.Write(
			ginContext,
			http.StatusForbidden,
			"membership_inactive",
			"Membership inactive",
			"The selected tenant does not have an active membership for this user.",
		)
	case errors.Is(err, identity.ErrTenantContextUnavailable):
		httpproblem.Write(
			ginContext,
			http.StatusServiceUnavailable,
			"tenant_context_unavailable",
			"Tenant context unavailable",
			"Tenant membership context is temporarily unavailable.",
		)
	default:
		httpproblem.InternalServerError(ginContext)
	}
}

func writeAuthenticationProblem(ginContext *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrAuthenticationRequired):
		httpproblem.Write(
			ginContext,
			http.StatusUnauthorized,
			"authentication_required",
			"Authentication required",
			"Supply a valid Kratos session cookie or session token.",
		)
	case errors.Is(err, identity.ErrInvalidSession):
		httpproblem.Write(
			ginContext,
			http.StatusUnauthorized,
			"invalid_session",
			"Invalid session",
			"The supplied session is invalid or expired.",
		)
	case errors.Is(err, identity.ErrUserDisabled):
		httpproblem.Write(
			ginContext,
			http.StatusForbidden,
			"user_disabled",
			"User disabled",
			"The application user is disabled.",
		)
	case errors.Is(err, identity.ErrIdentityMappingConflict):
		httpproblem.Write(
			ginContext,
			http.StatusConflict,
			"identity_mapping_conflict",
			"Identity mapping conflict",
			"The authenticated identity cannot be mapped safely.",
		)
	case errors.Is(err, identity.ErrAuthenticationUnavailable):
		httpproblem.Write(
			ginContext,
			http.StatusServiceUnavailable,
			"authentication_unavailable",
			"Authentication unavailable",
			"The authentication boundary is temporarily unavailable.",
		)
	default:
		httpproblem.InternalServerError(ginContext)
	}
}
