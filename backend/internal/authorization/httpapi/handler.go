package authorizationhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/credar/backend/internal/authorization"
	authorizationpostgres "github.com/lupppig/credar/backend/internal/authorization/postgres"
	"github.com/lupppig/credar/backend/internal/identity"
	identityhttp "github.com/lupppig/credar/backend/internal/identity/httpapi"
	"github.com/lupppig/credar/backend/internal/platform/httpproblem"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
	"github.com/lupppig/credar/backend/internal/tenancy"
	tenancypostgres "github.com/lupppig/credar/backend/internal/tenancy/postgres"
)

type Handler struct {
	identity      *identityhttp.Handler
	authorization *authorizationpostgres.Repository
	tenancy       *tenancypostgres.Repository
	service       *tenancy.Service
}

func New(identityHandler *identityhttp.Handler, authorizationRepository *authorizationpostgres.Repository, tenancyRepository *tenancypostgres.Repository, tenancyService *tenancy.Service) *Handler {
	return &Handler{identity: identityHandler, authorization: authorizationRepository, tenancy: tenancyRepository, service: tenancyService}
}

func (handler *Handler) RegisterRoutes(router *gin.Engine) {
	authenticated := router.Group("/v1", handler.identity.AuthenticationMiddleware())
	authenticated.GET("/tenants", handler.listTenants)
	authenticated.POST("/tenants", handler.createTenant)
	authenticated.PUT("/platform/users/:user_id/roles", handler.replacePlatformRoles)
	tenant := authenticated.Group("", handler.requireTenantContext())
	tenant.GET("/tenant", handler.getTenant)
	tenant.PATCH("/tenant", handler.updateTenant)
	tenant.GET("/memberships", handler.listMemberships)
	tenant.POST("/memberships", handler.createMembership)
	tenant.PATCH("/memberships/:membership_id", handler.updateMembershipStatus)
	tenant.PUT("/memberships/:membership_id/roles", handler.replaceMembershipRoles)
	tenant.GET("/roles", handler.listRoles)
	tenant.POST("/roles", handler.createRole)
	tenant.PATCH("/roles/:role_id", handler.updateRole)
	tenant.DELETE("/roles/:role_id", handler.deleteRole)
	tenant.GET("/audit-events", handler.listAudit)
}

func (handler *Handler) requireTenantContext() gin.HandlerFunc {
	return func(context *gin.Context) {
		value := context.GetHeader("X-Tenant-ID")
		if value == "" {
			problem(context, http.StatusBadRequest, "tenant_context_required", "Tenant context required", "Supply X-Tenant-ID for this operation.")
			return
		}
		if !identifier.IsUUIDv7(value) {
			problem(context, http.StatusBadRequest, "invalid_tenant_context", "Invalid tenant context", "X-Tenant-ID must be a canonical UUIDv7.")
			return
		}
		context.Request = context.Request.WithContext(requestcontext.WithTenantID(context.Request.Context(), value))
		context.Next()
	}
}

func (handler *Handler) listTenants(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	if err := handler.authorization.RequirePlatformPermission(context.Request.Context(), user.ID, "platform.tenants:read"); err != nil {
		writeError(context, err)
		return
	}
	items, err := handler.tenancy.ListTenants(context.Request.Context(), pageLimit(context))
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"data": items, "page": authorization.Page{HasMore: false}})
}

func (handler *Handler) createTenant(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	var request struct {
		Name        string `json:"name"`
		OwnerUserID string `json:"owner_user_id"`
	}
	if !decode(context, &request) {
		return
	}
	created, err := handler.service.CreateTenant(context.Request.Context(), user.ID, tenancy.CreateTenantCommand{Name: request.Name, OwnerUserID: request.OwnerUserID, IdempotencyKey: context.GetHeader("Idempotency-Key")})
	if err != nil {
		writeError(context, err)
		return
	}
	membership, err := handler.authorization.GetMembership(context.Request.Context(), created.Tenant.ID, created.InitialMembership.ID)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusCreated, gin.H{"tenant": created.Tenant, "owner_membership": membership})
}

func (handler *Handler) getTenant(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	tenantID := requestcontext.TenantID(context.Request.Context())
	if err := handler.authorization.RequireTenantPermission(context.Request.Context(), user.ID, tenantID, "tenants:read"); err != nil {
		writeError(context, err)
		return
	}
	item, err := handler.tenancy.GetTenant(context.Request.Context(), tenantID)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, item)
}

func (handler *Handler) updateTenant(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	tenantID := requestcontext.TenantID(context.Request.Context())
	if err := handler.authorization.RequireTenantPermission(context.Request.Context(), user.ID, tenantID, "tenants:manage"); err != nil {
		writeError(context, err)
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decode(context, &request) {
		return
	}
	item, err := handler.tenancy.UpdateTenant(context.Request.Context(), tenantID, request.Name)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, item)
}

func (handler *Handler) listMemberships(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	tenantID := requestcontext.TenantID(context.Request.Context())
	items, err := handler.authorization.ListMemberships(context.Request.Context(), user.ID, tenantID, pageLimit(context))
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"data": items, "page": authorization.Page{HasMore: false}})
}

func (handler *Handler) createMembership(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	tenantID := requestcontext.TenantID(context.Request.Context())
	var request struct {
		UserID  string   `json:"user_id"`
		RoleIDs []string `json:"role_ids"`
	}
	if !decode(context, &request) {
		return
	}
	created, err := handler.service.CreateMembership(context.Request.Context(), user.ID, tenancy.CreateMembershipCommand{TenantID: tenantID, UserID: request.UserID, RoleIDs: request.RoleIDs})
	if err != nil {
		writeError(context, err)
		return
	}
	item, err := handler.authorization.GetMembership(context.Request.Context(), tenantID, created.ID)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusCreated, item)
}

func (handler *Handler) updateMembershipStatus(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	tenantID := requestcontext.TenantID(context.Request.Context())
	var request struct {
		Status tenancy.MembershipStatus `json:"status"`
		Reason string                   `json:"reason"`
	}
	if !decode(context, &request) {
		return
	}
	changed, err := handler.service.ChangeMembershipStatus(context.Request.Context(), user.ID, tenancy.ChangeMembershipStatusCommand{TenantID: tenantID, MembershipID: context.Param("membership_id"), Status: request.Status, Reason: request.Reason})
	if err != nil {
		writeError(context, err)
		return
	}
	item, err := handler.authorization.GetMembership(context.Request.Context(), tenantID, changed.ID)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, item)
}

func (handler *Handler) replaceMembershipRoles(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	var request struct {
		RoleIDs []string `json:"role_ids"`
		Reason  string   `json:"reason"`
	}
	if !decode(context, &request) {
		return
	}
	item, err := handler.authorization.ReplaceMembershipRoles(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()), context.Param("membership_id"), request.RoleIDs, request.Reason)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, item)
}

func (handler *Handler) listRoles(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	items, err := handler.authorization.ListRoles(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()))
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"data": items})
}

func (handler *Handler) createRole(context *gin.Context) { handler.writeRole(context, false) }
func (handler *Handler) updateRole(context *gin.Context) { handler.writeRole(context, true) }
func (handler *Handler) writeRole(context *gin.Context, update bool) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	var request struct {
		Name            string   `json:"name"`
		PermissionNames []string `json:"permission_names"`
		Reason          string   `json:"reason"`
	}
	if !decode(context, &request) {
		return
	}
	write := authorization.RoleWrite{Name: request.Name, PermissionNames: request.PermissionNames, Reason: request.Reason}
	var item authorization.Role
	var err error
	if update {
		item, err = handler.authorization.UpdateRole(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()), context.Param("role_id"), write)
	} else {
		item, err = handler.authorization.CreateRole(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()), write)
	}
	if err != nil {
		writeError(context, err)
		return
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	context.JSON(status, item)
}

func (handler *Handler) deleteRole(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	err := handler.authorization.DeleteRole(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()), context.Param("role_id"))
	if err != nil {
		writeError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) listAudit(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	items, err := handler.authorization.ListAudit(context.Request.Context(), user.ID, requestcontext.TenantID(context.Request.Context()), pageLimit(context))
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"data": items, "page": authorization.Page{HasMore: false}})
}

func (handler *Handler) replacePlatformRoles(context *gin.Context) {
	user, ok := currentUser(context)
	if !ok {
		return
	}
	var request struct {
		RoleIDs []string `json:"role_ids"`
		Reason  string   `json:"reason"`
	}
	if !decode(context, &request) {
		return
	}
	targetID := context.Param("user_id")
	roles, err := handler.authorization.ReplacePlatformRoles(context.Request.Context(), user.ID, targetID, request.RoleIDs, request.Reason)
	if err != nil {
		writeError(context, err)
		return
	}
	target, err := handler.authorization.GetUser(context.Request.Context(), targetID)
	if err != nil {
		writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"user": target, "roles": roles})
}

func currentUser(context *gin.Context) (identity.User, bool) {
	user, ok := identityhttp.AuthenticatedUser(context)
	if !ok {
		httpproblem.InternalServerError(context)
	}
	return user, ok
}
func decode(context *gin.Context, target any) bool {
	decoder := json.NewDecoder(context.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		problem(context, http.StatusBadRequest, "invalid_request", "Invalid request", "The JSON request body is invalid.")
		return false
	}
	return true
}
func pageLimit(context *gin.Context) int {
	value := context.Query("page[limit]")
	if value == "" {
		return 50
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 200 {
		return 50
	}
	return limit
}

func writeError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrMembershipInactive):
		problem(context, http.StatusForbidden, "membership_inactive", "Membership inactive", "The selected tenant does not have an active membership for this user.")
	case errors.Is(err, authorization.ErrPermissionDenied):
		problem(context, http.StatusForbidden, "permission_denied", "Permission denied", "The authenticated user is not allowed to perform this operation.")
	case errors.Is(err, authorization.ErrResourceNotFound), errors.Is(err, tenancy.ErrTenantNotFound), errors.Is(err, tenancy.ErrMembershipNotFound), errors.Is(err, tenancy.ErrUserNotFound):
		problem(context, http.StatusNotFound, "resource_not_found", "Resource not found", "The resource does not exist in the selected scope.")
	case errors.Is(err, authorization.ErrSystemRoleImmutable):
		problem(context, http.StatusConflict, "system_role_immutable", "System role immutable", "System roles cannot be modified.")
	case errors.Is(err, authorization.ErrPermissionScopeEscalation):
		problem(context, http.StatusConflict, "permission_scope_escalation", "Permission scope escalation", "The requested role or permission is outside the tenant scope.")
	case errors.Is(err, authorization.ErrLastTenantOwnerRequired):
		problem(context, http.StatusConflict, "last_tenant_owner_required", "Last tenant owner required", "The tenant must retain at least one active owner.")
	case errors.Is(err, authorization.ErrLastPlatformAdminRequired):
		problem(context, http.StatusConflict, "last_platform_admin_required", "Last platform administrator required", "The platform must retain at least one active administrator.")
	case errors.Is(err, tenancy.ErrMembershipExists):
		problem(context, http.StatusConflict, "membership_already_exists", "Membership already exists", "A current membership already exists for this user.")
	case errors.Is(err, tenancy.ErrInvalidTransition):
		problem(context, http.StatusConflict, "invalid_membership_transition", "Invalid membership transition", "The requested membership status transition is not allowed.")
	case errors.Is(err, tenancy.ErrIdempotencyKeyReused):
		problem(context, http.StatusConflict, "idempotency_key_reused", "Idempotency key reused", "The idempotency key was used with different input.")
	case errors.Is(err, authorization.ErrRoleAssigned):
		problem(context, http.StatusConflict, "role_assigned", "Role assigned", "Remove this role from all memberships before deleting it.")
	case errors.Is(err, authorization.ErrInvalidInput), errors.Is(err, tenancy.ErrInvalidInput):
		problem(context, http.StatusBadRequest, "invalid_request", "Invalid request", "One or more request values are invalid.")
	case errors.Is(err, authorization.ErrUnavailable), errors.Is(err, tenancy.ErrUnavailable):
		problem(context, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization unavailable", "The authorization boundary is temporarily unavailable.")
	default:
		httpproblem.InternalServerError(context)
	}
}
func problem(context *gin.Context, status int, code, title, detail string) {
	httpproblem.Write(context, status, code, title, detail)
}
