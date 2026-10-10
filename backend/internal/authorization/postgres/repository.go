package authorizationpostgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/lupppig/credar/backend/internal/authorization"
	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

type EventInserter interface {
	Insert(context.Context, database.DBTX, event.Envelope) error
}

type Repository struct {
	database *database.Database
	events   EventInserter
}

func New(databaseConnection *database.Database, events EventInserter) *Repository {
	return &Repository{database: databaseConnection, events: events}
}

func (repository *Repository) RequirePlatformPermission(ctx context.Context, userID, permission string) error {
	if !identifier.IsUUIDv7(userID) || !strings.HasPrefix(permission, "platform.") {
		return authorization.ErrPermissionDenied
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return errors.Join(authorization.ErrUnavailable, err)
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM identity.users u
		JOIN identity.platform_role_assignments a ON a.user_id = u.id
		JOIN identity.role_permissions rp ON rp.role_id = a.role_id
		WHERE u.id = $1 AND u.status = 'ACTIVE' AND rp.permission_name = $2
	)`, userID, permission).Scan(&allowed)
	if err != nil {
		return errors.Join(authorization.ErrUnavailable, err)
	}
	if !allowed {
		return authorization.ErrPermissionDenied
	}
	return nil
}

func (repository *Repository) RequireTenantPermission(ctx context.Context, userID, tenantID, permission string) error {
	if !identifier.IsUUIDv7(userID) || !identifier.IsUUIDv7(tenantID) || strings.HasPrefix(permission, "platform.") {
		return authorization.ErrPermissionDenied
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return errors.Join(authorization.ErrUnavailable, err)
	}
	var activeMembership bool
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT
		EXISTS (
			SELECT 1 FROM identity.users u
			JOIN tenancy.memberships m ON m.user_id = u.id
			JOIN tenancy.tenants t ON t.id = m.tenant_id
			WHERE u.id = $1 AND u.status = 'ACTIVE' AND m.tenant_id = $2
			  AND m.status = 'ACTIVE' AND t.status = 'ACTIVE'
		),
		EXISTS (
			SELECT 1 FROM identity.users u
			JOIN tenancy.memberships m ON m.user_id = u.id
			JOIN tenancy.tenants t ON t.id = m.tenant_id
			JOIN identity.membership_role_assignments a ON a.membership_id = m.id
			JOIN identity.role_permissions rp ON rp.role_id = a.role_id
			WHERE u.id = $1 AND u.status = 'ACTIVE' AND m.tenant_id = $2
			  AND m.status = 'ACTIVE' AND t.status = 'ACTIVE' AND rp.permission_name = $3
		)`, userID, tenantID, permission).Scan(&activeMembership, &allowed)
	if err != nil {
		return errors.Join(authorization.ErrUnavailable, err)
	}
	if !activeMembership {
		return identity.ErrMembershipInactive
	}
	if !allowed {
		return authorization.ErrPermissionDenied
	}
	return nil
}

func (repository *Repository) BootstrapPlatformAdmin(ctx context.Context, external identity.ExternalIdentity) (identity.User, error) {
	if strings.TrimSpace(external.ID) == "" || strings.TrimSpace(external.Email) == "" {
		return identity.User{}, authorization.ErrInvalidInput
	}
	var user identity.User
	err := repository.inTransaction(ctx, func(tx database.DBTX) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE identity.platform_role_assignments IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM identity.platform_role_assignments`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return authorization.ErrBootstrapAlreadyComplete
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "identity:"+external.ID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,kratos_identity_id,email,status,created_at,updated_at FROM identity.users WHERE kratos_identity_id=$1 OR email=$2 ORDER BY id`, strings.ToLower(external.ID), strings.ToLower(external.Email))
		if err != nil {
			return err
		}
		var matches []identity.User
		for rows.Next() {
			var item identity.User
			if err := rows.Scan(&item.ID, &item.KratosIdentityID, &item.Email, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
				rows.Close()
				return err
			}
			matches = append(matches, item)
		}
		rows.Close()
		if len(matches) > 1 || (len(matches) == 1 && matches[0].KratosIdentityID != strings.ToLower(external.ID)) {
			return identity.ErrIdentityMappingConflict
		}
		if len(matches) == 1 {
			user = matches[0]
		} else {
			userID, err := identifier.NewUUIDv7()
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			user = identity.User{ID: userID, KratosIdentityID: strings.ToLower(external.ID), Email: strings.ToLower(external.Email), Status: identity.StatusActive, CreatedAt: now, UpdatedAt: now}
			if _, err := tx.Exec(ctx, `INSERT INTO identity.users(id,kratos_identity_id,email,status,created_at,updated_at) VALUES($1,$2,$3,'ACTIVE',$4,$4)`, user.ID, user.KratosIdentityID, user.Email, now); err != nil {
				return err
			}
			if err := repository.recordChange(ctx, tx, changeRecord{action: "identity.user_linked", actorType: "SYSTEM", targetType: "user", targetID: user.ID, changes: map[string]any{"user_id": user.ID}}); err != nil {
				return err
			}
		}
		if user.Status != identity.StatusActive {
			return identity.ErrUserDisabled
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.platform_role_assignments (user_id, role_id) VALUES ($1, $2)`, user.ID, authorization.PlatformAdminRoleID); err != nil {
			return err
		}
		return repository.recordChange(ctx, tx, changeRecord{
			action: "identity.platform_roles_changed", actorType: "SYSTEM", targetType: "user",
			targetID: user.ID, changes: map[string]any{"role_ids": []string{authorization.PlatformAdminRoleID}},
		})
	})
	return user, err
}

func (repository *Repository) ReplacePlatformRoles(ctx context.Context, actorID, userID string, roleIDs []string, reason string) ([]authorization.Role, error) {
	if err := repository.RequirePlatformPermission(ctx, actorID, "platform.memberships:manage"); err != nil {
		return nil, err
	}
	roleIDs = uniqueSorted(roleIDs)
	if !identifier.IsUUIDv7(userID) || len(roleIDs) > 1 || (len(roleIDs) == 1 && roleIDs[0] != authorization.PlatformAdminRoleID) {
		return nil, authorization.ErrInvalidInput
	}
	err := repository.inTransaction(ctx, func(tx database.DBTX) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE identity.platform_role_assignments IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1 AND status = 'ACTIVE')`, userID).Scan(&exists); err != nil || !exists {
			return authorization.ErrResourceNotFound
		}
		var hadAdmin bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.platform_role_assignments WHERE user_id = $1 AND role_id = $2)`, userID, authorization.PlatformAdminRoleID).Scan(&hadAdmin); err != nil {
			return err
		}
		if hadAdmin && len(roleIDs) == 0 {
			var activeAdmins int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM identity.platform_role_assignments a JOIN identity.users u ON u.id = a.user_id WHERE a.role_id = $1 AND u.status = 'ACTIVE'`, authorization.PlatformAdminRoleID).Scan(&activeAdmins); err != nil {
				return err
			}
			if activeAdmins <= 1 {
				return authorization.ErrLastPlatformAdminRequired
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM identity.platform_role_assignments WHERE user_id = $1`, userID); err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO identity.platform_role_assignments (user_id, role_id) VALUES ($1, $2)`, userID, roleID); err != nil {
				return err
			}
		}
		return repository.recordChange(ctx, tx, changeRecord{action: "identity.platform_roles_changed", actorID: actorID, targetType: "user", targetID: userID, reason: reason, changes: map[string]any{"role_ids": roleIDs}})
	})
	if err != nil {
		return nil, wrap(err)
	}
	return repository.rolesByIDs(ctx, roleIDs)
}

func (repository *Repository) ListRoles(ctx context.Context, actorID, tenantID string) ([]authorization.Role, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "roles:read"); err != nil {
		return nil, err
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, errors.Join(authorization.ErrUnavailable, err)
	}
	return scanRoles(ctx, pool, `WHERE r.scope = 'TENANT' AND (r.tenant_id IS NULL OR r.tenant_id = $1) ORDER BY r.kind DESC, LOWER(r.name), r.id`, tenantID)
}

func (repository *Repository) CreateRole(ctx context.Context, actorID, tenantID string, write authorization.RoleWrite) (authorization.Role, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "roles:manage"); err != nil {
		return authorization.Role{}, err
	}
	return repository.writeRole(ctx, actorID, tenantID, "", write)
}

func (repository *Repository) UpdateRole(ctx context.Context, actorID, tenantID, roleID string, write authorization.RoleWrite) (authorization.Role, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "roles:manage"); err != nil {
		return authorization.Role{}, err
	}
	return repository.writeRole(ctx, actorID, tenantID, roleID, write)
}

func (repository *Repository) writeRole(ctx context.Context, actorID, tenantID, roleID string, write authorization.RoleWrite) (authorization.Role, error) {
	write.Name = strings.TrimSpace(write.Name)
	write.PermissionNames = uniqueSorted(write.PermissionNames)
	if !identifier.IsUUIDv7(tenantID) || (roleID != "" && !identifier.IsUUIDv7(roleID)) || !validRoleName(write.Name) || len(write.PermissionNames) == 0 {
		return authorization.Role{}, authorization.ErrInvalidInput
	}
	var result authorization.Role
	creating := roleID == ""
	err := repository.inTransaction(ctx, func(tx database.DBTX) error {
		var permissionCount int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM identity.permissions WHERE scope = 'TENANT' AND name = ANY($1)`, write.PermissionNames).Scan(&permissionCount); err != nil {
			return err
		}
		if permissionCount != len(write.PermissionNames) {
			return authorization.ErrPermissionScopeEscalation
		}
		now := time.Now().UTC()
		if roleID == "" {
			var err error
			roleID, err = identifier.NewUUIDv7()
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO identity.roles (id, tenant_id, name, scope, kind, created_at, updated_at) VALUES ($1,$2,$3,'TENANT','CUSTOM',$4,$4)`, roleID, tenantID, write.Name, now)
			if err != nil {
				return err
			}
		} else {
			var kind string
			err := tx.QueryRow(ctx, `SELECT kind FROM identity.roles WHERE id = $1 AND (tenant_id = $2 OR tenant_id IS NULL) FOR UPDATE`, roleID, tenantID).Scan(&kind)
			if errors.Is(err, pgx.ErrNoRows) {
				return authorization.ErrResourceNotFound
			}
			if err != nil {
				return err
			}
			if kind == "SYSTEM" {
				return authorization.ErrSystemRoleImmutable
			}
			if _, err := tx.Exec(ctx, `UPDATE identity.roles SET name=$3, updated_at=$4 WHERE id=$1 AND tenant_id=$2`, roleID, tenantID, write.Name, now); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM identity.role_permissions WHERE role_id=$1`, roleID); err != nil {
				return err
			}
		}
		for _, permission := range write.PermissionNames {
			if _, err := tx.Exec(ctx, `INSERT INTO identity.role_permissions (role_id, permission_name) VALUES ($1,$2)`, roleID, permission); err != nil {
				return err
			}
		}
		action := "role.created"
		if !creating {
			action = "role.updated"
		}
		if err := repository.recordChange(ctx, tx, changeRecord{action: action, actorID: actorID, tenantID: &tenantID, targetType: "role", targetID: roleID, reason: write.Reason, changes: map[string]any{"name": write.Name, "permission_names": write.PermissionNames}}); err != nil {
			return err
		}
		result = authorization.Role{ID: roleID, TenantID: &tenantID, Name: write.Name, Scope: authorization.ScopeTenant, Kind: authorization.KindCustom, CreatedAt: now, UpdatedAt: now}
		return nil
	})
	if err != nil {
		return authorization.Role{}, wrap(err)
	}
	roles, err := repository.rolesByIDs(ctx, []string{result.ID})
	if err != nil || len(roles) != 1 {
		return authorization.Role{}, err
	}
	return roles[0], nil
}

func (repository *Repository) DeleteRole(ctx context.Context, actorID, tenantID, roleID string) error {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "roles:manage"); err != nil {
		return err
	}
	if !identifier.IsUUIDv7(roleID) {
		return authorization.ErrInvalidInput
	}
	return repository.inTransaction(ctx, func(tx database.DBTX) error {
		var kind, name string
		err := tx.QueryRow(ctx, `SELECT kind,name FROM identity.roles WHERE id=$1 AND (tenant_id=$2 OR tenant_id IS NULL) FOR UPDATE`, roleID, tenantID).Scan(&kind, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ErrResourceNotFound
		}
		if err != nil {
			return err
		}
		if kind == "SYSTEM" {
			return authorization.ErrSystemRoleImmutable
		}
		var assigned bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_role_assignments WHERE role_id=$1)`, roleID).Scan(&assigned); err != nil {
			return err
		}
		if assigned {
			return authorization.ErrRoleAssigned
		}
		rows, err := tx.Query(ctx, `SELECT permission_name FROM identity.role_permissions WHERE role_id=$1 ORDER BY permission_name`, roleID)
		if err != nil {
			return err
		}
		var permissionNames []string
		for rows.Next() {
			var permission string
			if err := rows.Scan(&permission); err != nil {
				rows.Close()
				return err
			}
			permissionNames = append(permissionNames, permission)
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `DELETE FROM identity.roles WHERE id=$1 AND tenant_id=$2`, roleID, tenantID); err != nil {
			return err
		}
		return repository.recordChange(ctx, tx, changeRecord{action: "role.deleted", actorID: actorID, tenantID: &tenantID, targetType: "role", targetID: roleID, changes: map[string]any{"name": name, "permission_names": permissionNames}})
	})
}

func (repository *Repository) ReplaceMembershipRoles(ctx context.Context, actorID, tenantID, membershipID string, roleIDs []string, reason string) (authorization.Membership, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "roles:manage"); err != nil {
		return authorization.Membership{}, err
	}
	roleIDs = uniqueSorted(roleIDs)
	if !identifier.IsUUIDv7(membershipID) || len(roleIDs) == 0 {
		return authorization.Membership{}, authorization.ErrInvalidInput
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return authorization.Membership{}, wrap(err)
	}
	var currentlyOwner bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_role_assignments WHERE membership_id=$1 AND role_id=$2)`, membershipID, authorization.TenantOwnerRoleID).Scan(&currentlyOwner); err != nil {
		return authorization.Membership{}, wrap(err)
	}
	if currentlyOwner || contains(roleIDs, authorization.TenantOwnerRoleID) {
		if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "tenants:manage"); err != nil {
			return authorization.Membership{}, err
		}
	}
	err = repository.inTransaction(ctx, func(tx database.DBTX) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM tenancy.memberships WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, membershipID, tenantID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) || status == "REMOVED" {
			return authorization.ErrResourceNotFound
		}
		if err != nil {
			return err
		}
		valid, err := validateTenantRoles(ctx, tx, tenantID, roleIDs)
		if err != nil {
			return err
		}
		if !valid {
			return authorization.ErrPermissionScopeEscalation
		}
		var membershipUserID string
		if err := tx.QueryRow(ctx, `SELECT user_id FROM tenancy.memberships WHERE id=$1 AND tenant_id=$2`, membershipID, tenantID).Scan(&membershipUserID); err != nil {
			return err
		}
		var hadOwner bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_role_assignments WHERE membership_id=$1 AND role_id=$2)`, membershipID, authorization.TenantOwnerRoleID).Scan(&hadOwner); err != nil {
			return err
		}
		if hadOwner && !contains(roleIDs, authorization.TenantOwnerRoleID) && status == "ACTIVE" {
			if err := requireAnotherOwner(ctx, tx, tenantID, membershipID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM identity.membership_role_assignments WHERE membership_id=$1`, membershipID); err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO identity.membership_role_assignments (membership_id, role_id) VALUES ($1,$2)`, membershipID, roleID); err != nil {
				return err
			}
		}
		return repository.recordChange(ctx, tx, changeRecord{action: "membership.roles_changed", actorID: actorID, tenantID: &tenantID, targetType: "membership", targetID: membershipID, reason: reason, changes: map[string]any{"user_id": membershipUserID, "role_ids": roleIDs}})
	})
	if err != nil {
		return authorization.Membership{}, wrap(err)
	}
	return repository.GetMembership(ctx, tenantID, membershipID)
}

func requireAnotherOwner(ctx context.Context, tx database.DBTX, tenantID, excludedMembershipID string) error {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM tenancy.memberships m JOIN identity.membership_role_assignments a ON a.membership_id=m.id WHERE m.tenant_id=$1 AND m.id<>$2 AND m.status='ACTIVE' AND a.role_id=$3`, tenantID, excludedMembershipID, authorization.TenantOwnerRoleID).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return authorization.ErrLastTenantOwnerRequired
	}
	return nil
}

func (repository *Repository) GetMembership(ctx context.Context, tenantID, membershipID string) (authorization.Membership, error) {
	pool, err := repository.database.Pool()
	if err != nil {
		return authorization.Membership{}, errors.Join(authorization.ErrUnavailable, err)
	}
	var item authorization.Membership
	err = pool.QueryRow(ctx, `SELECT m.id,m.tenant_id,m.status,m.created_at,m.updated_at,m.disabled_at,m.removed_at,u.id,u.kratos_identity_id,u.email,u.status,u.created_at,u.updated_at FROM tenancy.memberships m JOIN identity.users u ON u.id=m.user_id WHERE m.id=$1 AND m.tenant_id=$2`, membershipID, tenantID).Scan(&item.ID, &item.TenantID, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DisabledAt, &item.RemovedAt, &item.User.ID, &item.User.KratosIdentityID, &item.User.Email, &item.User.Status, &item.User.CreatedAt, &item.User.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.Membership{}, authorization.ErrResourceNotFound
	}
	if err != nil {
		return authorization.Membership{}, wrap(err)
	}
	item.Roles, err = repository.membershipRoles(ctx, item.ID)
	return item, err
}

func (repository *Repository) GetUser(ctx context.Context, userID string) (identity.User, error) {
	if !identifier.IsUUIDv7(userID) {
		return identity.User{}, authorization.ErrResourceNotFound
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return identity.User{}, wrap(err)
	}
	var user identity.User
	err = pool.QueryRow(ctx, `SELECT id,kratos_identity_id,email,status,created_at,updated_at FROM identity.users WHERE id=$1`, userID).Scan(&user.ID, &user.KratosIdentityID, &user.Email, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, authorization.ErrResourceNotFound
	}
	return user, wrap(err)
}

func (repository *Repository) ListMemberships(ctx context.Context, actorID, tenantID string, limit int) ([]authorization.Membership, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "memberships:read"); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, wrap(err)
	}
	rows, err := pool.Query(ctx, `SELECT m.id FROM tenancy.memberships m WHERE m.tenant_id=$1 ORDER BY m.created_at,m.id LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, wrap(err)
		}
		ids = append(ids, id)
	}
	items := make([]authorization.Membership, 0, len(ids))
	for _, id := range ids {
		item, err := repository.GetMembership(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (repository *Repository) ListAudit(ctx context.Context, actorID, tenantID string, limit int) ([]authorization.AuditEvent, error) {
	if err := repository.RequireTenantPermission(ctx, actorID, tenantID, "audit:read"); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, wrap(err)
	}
	rows, err := pool.Query(ctx, `SELECT id,action,actor_type,actor_id,tenant_id,target_type,target_id,occurred_at,correlation_id,reason,changes FROM identity.audit_events WHERE tenant_id=$1 ORDER BY occurred_at DESC,id DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	items := make([]authorization.AuditEvent, 0)
	for rows.Next() {
		var item authorization.AuditEvent
		var changes []byte
		if err := rows.Scan(&item.ID, &item.Action, &item.Actor.Type, &item.Actor.ID, &item.TenantID, &item.TargetType, &item.TargetID, &item.OccurredAt, &item.CorrelationID, &item.Reason, &changes); err != nil {
			return nil, wrap(err)
		}
		_ = json.Unmarshal(changes, &item.Changes)
		items = append(items, item)
	}
	return items, wrap(rows.Err())
}

func (repository *Repository) rolesByIDs(ctx context.Context, ids []string) ([]authorization.Role, error) {
	if len(ids) == 0 {
		return []authorization.Role{}, nil
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, wrap(err)
	}
	return scanRoles(ctx, pool, `WHERE r.id = ANY($1) ORDER BY LOWER(r.name),r.id`, ids)
}

func (repository *Repository) membershipRoles(ctx context.Context, membershipID string) ([]authorization.Role, error) {
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, wrap(err)
	}
	return scanRoles(ctx, pool, `JOIN identity.membership_role_assignments a ON a.role_id=r.id WHERE a.membership_id=$1 ORDER BY LOWER(r.name),r.id`, membershipID)
}

func scanRoles(ctx context.Context, db database.DBTX, clause string, args ...any) ([]authorization.Role, error) {
	rows, err := db.Query(ctx, `SELECT r.id,r.tenant_id,r.name,r.scope,r.kind,r.created_at,r.updated_at,p.name,p.scope,p.description FROM identity.roles r LEFT JOIN identity.role_permissions rp ON rp.role_id=r.id LEFT JOIN identity.permissions p ON p.name=rp.permission_name `+clause, args...)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	byID := map[string]*authorization.Role{}
	order := []string{}
	for rows.Next() {
		var base authorization.Role
		var pn, ps, pd *string
		if err := rows.Scan(&base.ID, &base.TenantID, &base.Name, &base.Scope, &base.Kind, &base.CreatedAt, &base.UpdatedAt, &pn, &ps, &pd); err != nil {
			return nil, wrap(err)
		}
		role := byID[base.ID]
		if role == nil {
			base.Permissions = []authorization.Permission{}
			role = &base
			byID[base.ID] = role
			order = append(order, base.ID)
		}
		if pn != nil {
			role.Permissions = append(role.Permissions, authorization.Permission{Name: *pn, Scope: authorization.Scope(*ps), Description: *pd})
		}
	}
	result := make([]authorization.Role, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, wrap(rows.Err())
}

func validateTenantRoles(ctx context.Context, tx database.DBTX, tenantID string, ids []string) (bool, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM identity.roles WHERE id=ANY($1) AND scope='TENANT' AND (tenant_id IS NULL OR tenant_id=$2)`, ids, tenantID).Scan(&count)
	return count == len(ids), err
}

type changeRecord struct {
	action, actorType, actorID, targetType, targetID, reason string
	tenantID                                                 *string
	changes                                                  map[string]any
}

func (repository *Repository) recordChange(ctx context.Context, tx database.DBTX, c changeRecord) error {
	now := time.Now().UTC()
	auditID, err := identifier.NewUUIDv7()
	if err != nil {
		return err
	}
	requestID := requestcontext.RequestID(ctx)
	correlationID := requestcontext.CorrelationID(ctx)
	if correlationID == "" {
		correlationID = auditID
	}
	actorType := c.actorType
	if actorType == "" {
		actorType = "USER"
	}
	var actorID *string
	if c.actorID != "" {
		actorID = &c.actorID
	}
	encoded, err := json.Marshal(auditChanges(c.changes))
	if err != nil {
		return err
	}
	var reason *string
	if strings.TrimSpace(c.reason) != "" {
		trimmed := strings.TrimSpace(c.reason)
		reason = &trimmed
	}
	_, err = tx.Exec(ctx, `INSERT INTO identity.audit_events(id,action,actor_type,actor_id,tenant_id,target_type,target_id,request_id,correlation_id,source,reason,changes,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, auditID, c.action, actorType, actorID, c.tenantID, c.targetType, c.targetID, nullUUID(requestID), correlationID, map[bool]string{true: "SYSTEM", false: "HTTP"}[actorType == "SYSTEM"], reason, encoded, now)
	if err != nil {
		return err
	}
	eventID, err := identifier.NewUUIDv7()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(eventData(c))
	return repository.events.Insert(ctx, tx, event.Envelope{ID: eventID, Type: eventType(c.action), Version: 1, OccurredAt: now, TenantID: c.tenantID, Aggregate: event.Aggregate{Type: c.targetType, ID: c.targetID}, CorrelationID: correlationID, CausationID: requestID, Data: data})
}

func eventType(action string) string {
	if strings.HasPrefix(action, "role.") {
		return "role.changed"
	}
	return action
}

func eventData(change changeRecord) map[string]any {
	switch change.action {
	case "identity.user_linked":
		return map[string]any{"user_id": change.targetID}
	case "identity.platform_roles_changed":
		return map[string]any{"user_id": change.targetID, "role_ids": change.changes["role_ids"]}
	case "membership.roles_changed":
		return map[string]any{"membership_id": change.targetID, "user_id": change.changes["user_id"], "role_ids": change.changes["role_ids"]}
	case "role.created", "role.updated", "role.deleted":
		verb := strings.ToUpper(strings.TrimPrefix(change.action, "role."))
		return map[string]any{"role_id": change.targetID, "name": change.changes["name"], "change": verb, "permission_names": change.changes["permission_names"]}
	default:
		return change.changes
	}
}

func auditChanges(changes map[string]any) map[string]any {
	result := make(map[string]any, len(changes))
	for name, value := range changes {
		switch typed := value.(type) {
		case []string:
			result[name] = strings.Join(typed, ",")
		default:
			result[name] = value
		}
	}
	return result
}
func nullUUID(v string) any {
	if identifier.IsUUIDv7(v) {
		return v
	}
	return nil
}
func (repository *Repository) inTransaction(ctx context.Context, operation func(database.DBTX) error) error {
	err := repository.database.InTransaction(ctx, pgx.TxOptions{}, operation)
	return wrap(err)
}
func wrap(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{authorization.ErrPermissionDenied, authorization.ErrResourceNotFound, authorization.ErrInvalidInput, authorization.ErrSystemRoleImmutable, authorization.ErrPermissionScopeEscalation, authorization.ErrLastTenantOwnerRequired, authorization.ErrLastPlatformAdminRequired, authorization.ErrRoleAssigned, authorization.ErrBootstrapAlreadyComplete, identity.ErrMembershipInactive} {
		if errors.Is(err, known) {
			return err
		}
	}
	return errors.Join(authorization.ErrUnavailable, err)
}
func uniqueSorted(values []string) []string {
	set := map[string]struct{}{}
	for _, v := range values {
		if identifier.IsUUIDv7(v) || strings.Contains(v, ":") {
			set[v] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}
func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func validRoleName(value string) bool {
	n := utf8.RuneCountInString(value)
	if n < 1 || n > 100 {
		return false
	}
	for i, r := range value {
		if i == 0 && !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
