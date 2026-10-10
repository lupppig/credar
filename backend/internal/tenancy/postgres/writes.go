package tenancypostgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/lupppig/credar/backend/internal/authorization"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
	"github.com/lupppig/credar/backend/internal/tenancy"
)

const (
	createTenantOperation = "tenant.create"
	idempotencyRetention  = 24 * time.Hour
)

func (repository *Repository) CreateTenant(
	ctx context.Context,
	command tenancy.CreateTenantCommand,
) (tenancy.CreatedTenant, error) {
	actorUserID, requestID, correlationID, err := writeContext(ctx)
	if err != nil {
		return tenancy.CreatedTenant{}, err
	}
	command.Name = strings.TrimSpace(command.Name)
	if !validName(command.Name) ||
		!identifier.IsUUIDv7(command.OwnerUserID) ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return tenancy.CreatedTenant{}, tenancy.ErrInvalidInput
	}
	fingerprint, err := tenantFingerprint(command)
	if err != nil {
		return tenancy.CreatedTenant{}, wrapPersistenceError(err)
	}

	var result tenancy.CreatedTenant
	err = repository.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if err := lockKey(
			ctx,
			transaction,
			"idempotency:"+actorUserID+":"+createTenantOperation+":"+command.IdempotencyKey,
		); err != nil {
			return err
		}

		found, err := findIdempotentTenant(
			ctx,
			transaction,
			actorUserID,
			command.IdempotencyKey,
			fingerprint,
			&result,
		)
		if err != nil || found {
			return err
		}
		if err := requireActiveUser(ctx, transaction, command.OwnerUserID); err != nil {
			return err
		}

		result, err = repository.insertTenant(
			ctx,
			transaction,
			command,
			actorUserID,
			requestID,
			correlationID,
		)
		if err != nil {
			return err
		}

		responseBody, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("marshal idempotent tenant response: %w", err)
		}
		_, err = transaction.Exec(
			ctx,
			`INSERT INTO platform.idempotency_records (
                actor_user_id, scope_tenant_id, operation, idempotency_key,
                request_hash, response_status, response_body, created_at, expires_at
            ) VALUES ($1, $2, $3, $4, $5, 201, $6, $7, $8)`,
			actorUserID,
			result.Tenant.ID,
			createTenantOperation,
			command.IdempotencyKey,
			fingerprint,
			responseBody,
			result.Tenant.CreatedAt,
			result.Tenant.CreatedAt.Add(idempotencyRetention),
		)
		if err != nil {
			return fmt.Errorf("insert tenant idempotency record: %w", err)
		}
		return nil
	})
	if err != nil {
		return tenancy.CreatedTenant{}, wrapPersistenceError(err)
	}
	return result, nil
}

func findIdempotentTenant(
	ctx context.Context,
	transaction database.DBTX,
	actorUserID string,
	idempotencyKey string,
	fingerprint string,
	result *tenancy.CreatedTenant,
) (bool, error) {
	var storedFingerprint string
	var responseBody []byte
	var expiresAt time.Time
	err := transaction.QueryRow(
		ctx,
		`SELECT request_hash, response_body, expires_at
         FROM platform.idempotency_records
         WHERE actor_user_id = $1 AND operation = $2 AND idempotency_key = $3
         FOR UPDATE`,
		actorUserID,
		createTenantOperation,
		idempotencyKey,
	).Scan(&storedFingerprint, &responseBody, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query tenant idempotency record: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		if _, err := transaction.Exec(
			ctx,
			`DELETE FROM platform.idempotency_records
             WHERE actor_user_id = $1 AND operation = $2 AND idempotency_key = $3`,
			actorUserID,
			createTenantOperation,
			idempotencyKey,
		); err != nil {
			return false, fmt.Errorf("delete expired tenant idempotency record: %w", err)
		}
		return false, nil
	}
	if storedFingerprint != fingerprint {
		return false, tenancy.ErrIdempotencyKeyReused
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return false, fmt.Errorf("decode tenant idempotency response: %w", err)
	}
	return true, nil
}

func (repository *Repository) insertTenant(
	ctx context.Context,
	transaction database.DBTX,
	command tenancy.CreateTenantCommand,
	actorUserID string,
	requestID string,
	correlationID string,
) (tenancy.CreatedTenant, error) {
	tenantID, err := identifier.NewUUIDv7()
	if err != nil {
		return tenancy.CreatedTenant{}, fmt.Errorf("create tenant ID: %w", err)
	}
	membershipID, err := identifier.NewUUIDv7()
	if err != nil {
		return tenancy.CreatedTenant{}, fmt.Errorf("create initial membership ID: %w", err)
	}
	now := time.Now().UTC()
	result := tenancy.CreatedTenant{
		Tenant: tenancy.Tenant{
			ID:        tenantID,
			Name:      command.Name,
			Status:    tenancy.TenantStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		InitialMembership: tenancy.Membership{
			ID:        membershipID,
			TenantID:  tenantID,
			UserID:    command.OwnerUserID,
			Status:    tenancy.MembershipStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO tenancy.tenants (id, name, status, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $4)`,
		result.Tenant.ID,
		result.Tenant.Name,
		result.Tenant.Status,
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, fmt.Errorf("insert tenant: %w", err)
	}
	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO tenancy.memberships (
            id, tenant_id, user_id, status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $5)`,
		result.InitialMembership.ID,
		result.InitialMembership.TenantID,
		result.InitialMembership.UserID,
		result.InitialMembership.Status,
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, fmt.Errorf("insert initial membership: %w", err)
	}
	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO identity.membership_role_assignments (membership_id, role_id)
		 VALUES ($1, $2)`,
		result.InitialMembership.ID,
		authorization.TenantOwnerRoleID,
	); err != nil {
		return tenancy.CreatedTenant{}, fmt.Errorf("assign initial owner role: %w", err)
	}

	if err := insertAudit(
		ctx,
		transaction,
		"tenant.created",
		actorUserID,
		&tenantID,
		"tenant",
		tenantID,
		requestID,
		correlationID,
		"",
		map[string]any{"name": command.Name},
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, err
	}
	if err := insertAudit(
		ctx,
		transaction,
		"membership.created",
		actorUserID,
		&tenantID,
		"membership",
		membershipID,
		requestID,
		correlationID,
		"",
		map[string]any{"status": tenancy.MembershipStatusActive, "user_id": command.OwnerUserID},
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, err
	}

	tenantData := struct {
		TenantID        string `json:"tenant_id"`
		Name            string `json:"name"`
		CreatedByUserID string `json:"created_by_user_id"`
	}{TenantID: tenantID, Name: command.Name, CreatedByUserID: actorUserID}
	if err := repository.insertEvent(
		ctx,
		transaction,
		"tenant.created",
		"tenant",
		tenantID,
		tenantID,
		requestID,
		correlationID,
		tenantData,
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, err
	}
	membershipData := membershipStatusEventData{
		MembershipID:   membershipID,
		UserID:         command.OwnerUserID,
		PreviousStatus: nil,
		Status:         tenancy.MembershipStatusActive,
	}
	if err := repository.insertEvent(
		ctx,
		transaction,
		"membership.status_changed",
		"membership",
		membershipID,
		tenantID,
		requestID,
		correlationID,
		membershipData,
		now,
	); err != nil {
		return tenancy.CreatedTenant{}, err
	}
	return result, nil
}

func (repository *Repository) CreateMembership(
	ctx context.Context,
	command tenancy.CreateMembershipCommand,
) (tenancy.Membership, error) {
	actorUserID, requestID, correlationID, err := writeContext(ctx)
	if err != nil {
		return tenancy.Membership{}, err
	}
	if !identifier.IsUUIDv7(command.TenantID) || !identifier.IsUUIDv7(command.UserID) || len(command.RoleIDs) == 0 {
		return tenancy.Membership{}, tenancy.ErrInvalidInput
	}

	var membership tenancy.Membership
	err = repository.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if err := lockKey(ctx, transaction, "membership:"+command.TenantID+":"+command.UserID); err != nil {
			return err
		}
		if err := requireActiveTenantAndUser(ctx, transaction, command.TenantID, command.UserID); err != nil {
			return err
		}
		roleIDs := uniqueRoleIDs(command.RoleIDs)
		if len(roleIDs) != len(command.RoleIDs) {
			return tenancy.ErrInvalidInput
		}
		var roleCount int
		if err := transaction.QueryRow(
			ctx,
			`SELECT COUNT(*) FROM identity.roles
			 WHERE id = ANY($1) AND scope = 'TENANT'
			   AND (tenant_id IS NULL OR tenant_id = $2)`,
			roleIDs,
			command.TenantID,
		).Scan(&roleCount); err != nil {
			return fmt.Errorf("validate membership roles: %w", err)
		}
		if roleCount != len(roleIDs) {
			return authorization.ErrPermissionScopeEscalation
		}
		var exists bool
		if err := transaction.QueryRow(
			ctx,
			`SELECT EXISTS (
                SELECT 1 FROM tenancy.memberships
                WHERE tenant_id = $1 AND user_id = $2 AND status <> 'REMOVED'
            )`,
			command.TenantID,
			command.UserID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("query current membership: %w", err)
		}
		if exists {
			return tenancy.ErrMembershipExists
		}

		membershipID, err := identifier.NewUUIDv7()
		if err != nil {
			return fmt.Errorf("create membership ID: %w", err)
		}
		now := time.Now().UTC()
		membership = tenancy.Membership{
			ID:        membershipID,
			TenantID:  command.TenantID,
			UserID:    command.UserID,
			Status:    tenancy.MembershipStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := transaction.Exec(
			ctx,
			`INSERT INTO tenancy.memberships (
                id, tenant_id, user_id, status, created_at, updated_at
            ) VALUES ($1, $2, $3, $4, $5, $5)`,
			membership.ID,
			membership.TenantID,
			membership.UserID,
			membership.Status,
			now,
		); err != nil {
			return fmt.Errorf("insert membership: %w", err)
		}
		for _, roleID := range roleIDs {
			if _, err := transaction.Exec(
				ctx,
				`INSERT INTO identity.membership_role_assignments (membership_id, role_id)
				 VALUES ($1, $2)`,
				membership.ID,
				roleID,
			); err != nil {
				return fmt.Errorf("assign membership role: %w", err)
			}
		}
		if err := insertAudit(
			ctx,
			transaction,
			"membership.created",
			actorUserID,
			&command.TenantID,
			"membership",
			membership.ID,
			requestID,
			correlationID,
			"",
			map[string]any{"status": membership.Status, "user_id": membership.UserID},
			now,
		); err != nil {
			return err
		}
		return repository.insertEvent(
			ctx,
			transaction,
			"membership.status_changed",
			"membership",
			membership.ID,
			membership.TenantID,
			requestID,
			correlationID,
			membershipStatusEventData{
				MembershipID:   membership.ID,
				UserID:         membership.UserID,
				PreviousStatus: nil,
				Status:         membership.Status,
			},
			now,
		)
	})
	if err != nil {
		return tenancy.Membership{}, wrapPersistenceError(err)
	}
	return membership, nil
}

func (repository *Repository) ChangeMembershipStatus(
	ctx context.Context,
	command tenancy.ChangeMembershipStatusCommand,
) (tenancy.Membership, error) {
	actorUserID, requestID, correlationID, err := writeContext(ctx)
	if err != nil {
		return tenancy.Membership{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if !identifier.IsUUIDv7(command.TenantID) ||
		!identifier.IsUUIDv7(command.MembershipID) ||
		(command.Reason != "" && utf8.RuneCountInString(command.Reason) > 2000) {
		return tenancy.Membership{}, tenancy.ErrInvalidInput
	}

	var membership tenancy.Membership
	var previousStatus tenancy.MembershipStatus
	err = repository.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if err := transaction.QueryRow(
			ctx,
			`SELECT id, tenant_id, user_id, status, created_at, updated_at,
                    disabled_at, removed_at
             FROM tenancy.memberships
             WHERE id = $1 AND tenant_id = $2
             FOR UPDATE`,
			command.MembershipID,
			command.TenantID,
		).Scan(
			&membership.ID,
			&membership.TenantID,
			&membership.UserID,
			&membership.Status,
			&membership.CreatedAt,
			&membership.UpdatedAt,
			&membership.DisabledAt,
			&membership.RemovedAt,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return tenancy.ErrMembershipNotFound
			}
			return fmt.Errorf("query membership for status change: %w", err)
		}

		previousStatus = membership.Status
		if !tenancy.CanTransitionMembership(previousStatus, command.Status) {
			return tenancy.ErrInvalidTransition
		}
		if previousStatus == tenancy.MembershipStatusActive &&
			command.Status != tenancy.MembershipStatusActive {
			var isOwner bool
			if err := transaction.QueryRow(
				ctx,
				`SELECT EXISTS (
					SELECT 1 FROM identity.membership_role_assignments
					WHERE membership_id = $1 AND role_id = $2
				)`,
				membership.ID,
				authorization.TenantOwnerRoleID,
			).Scan(&isOwner); err != nil {
				return fmt.Errorf("query owner assignment: %w", err)
			}
			if isOwner {
				var otherOwners int
				if err := transaction.QueryRow(
					ctx,
					`SELECT COUNT(*)
					 FROM tenancy.memberships memberships
					 JOIN identity.membership_role_assignments assignments
					   ON assignments.membership_id = memberships.id
					 WHERE memberships.tenant_id = $1
					   AND memberships.id <> $2
					   AND memberships.status = 'ACTIVE'
					   AND assignments.role_id = $3`,
					membership.TenantID,
					membership.ID,
					authorization.TenantOwnerRoleID,
				).Scan(&otherOwners); err != nil {
					return fmt.Errorf("count active tenant owners: %w", err)
				}
				if otherOwners == 0 {
					return authorization.ErrLastTenantOwnerRequired
				}
			}
		}
		now := time.Now().UTC()
		membership.Status = command.Status
		membership.UpdatedAt = now
		switch command.Status {
		case tenancy.MembershipStatusActive:
			membership.DisabledAt = nil
			membership.RemovedAt = nil
		case tenancy.MembershipStatusDisabled:
			membership.DisabledAt = &now
			membership.RemovedAt = nil
		case tenancy.MembershipStatusRemoved:
			membership.RemovedAt = &now
		default:
			return tenancy.ErrInvalidInput
		}

		if _, err := transaction.Exec(
			ctx,
			`UPDATE tenancy.memberships
             SET status = $3, updated_at = $4, disabled_at = $5, removed_at = $6
             WHERE id = $1 AND tenant_id = $2`,
			membership.ID,
			membership.TenantID,
			membership.Status,
			membership.UpdatedAt,
			membership.DisabledAt,
			membership.RemovedAt,
		); err != nil {
			return fmt.Errorf("update membership status: %w", err)
		}
		if err := insertAudit(
			ctx,
			transaction,
			"membership.status_changed",
			actorUserID,
			&membership.TenantID,
			"membership",
			membership.ID,
			requestID,
			correlationID,
			command.Reason,
			map[string]any{"previous_status": previousStatus, "status": membership.Status},
			now,
		); err != nil {
			return err
		}
		return repository.insertEvent(
			ctx,
			transaction,
			"membership.status_changed",
			"membership",
			membership.ID,
			membership.TenantID,
			requestID,
			correlationID,
			membershipStatusEventData{
				MembershipID:   membership.ID,
				UserID:         membership.UserID,
				PreviousStatus: &previousStatus,
				Status:         membership.Status,
			},
			now,
		)
	})
	if err != nil {
		return tenancy.Membership{}, wrapPersistenceError(err)
	}
	return membership, nil
}

type membershipStatusEventData struct {
	MembershipID   string                    `json:"membership_id"`
	UserID         string                    `json:"user_id"`
	PreviousStatus *tenancy.MembershipStatus `json:"previous_status"`
	Status         tenancy.MembershipStatus  `json:"status"`
}

func (repository *Repository) insertEvent(
	ctx context.Context,
	transaction database.DBTX,
	eventType string,
	aggregateType string,
	aggregateID string,
	tenantID string,
	requestID string,
	correlationID string,
	data any,
	occurredAt time.Time,
) error {
	eventID, err := identifier.NewUUIDv7()
	if err != nil {
		return fmt.Errorf("create integration event ID: %w", err)
	}
	encodedData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", eventType, err)
	}
	return repository.events.Insert(ctx, transaction, event.Envelope{
		ID:         eventID,
		Type:       eventType,
		Version:    1,
		OccurredAt: occurredAt,
		TenantID:   &tenantID,
		Aggregate: event.Aggregate{
			Type: aggregateType,
			ID:   aggregateID,
		},
		CorrelationID: correlationID,
		CausationID:   requestID,
		Data:          encodedData,
	})
}

func insertAudit(
	ctx context.Context,
	transaction database.DBTX,
	action string,
	actorUserID string,
	tenantID *string,
	targetType string,
	targetID string,
	requestID string,
	correlationID string,
	reason string,
	changes map[string]any,
	occurredAt time.Time,
) error {
	auditID, err := identifier.NewUUIDv7()
	if err != nil {
		return fmt.Errorf("create audit event ID: %w", err)
	}
	encodedChanges, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("marshal audit changes: %w", err)
	}
	var nullableReason *string
	if reason != "" {
		nullableReason = &reason
	}
	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO identity.audit_events (
            id, action, actor_type, actor_id, tenant_id, target_type, target_id,
            request_id, correlation_id, source, reason, changes, occurred_at
        ) VALUES ($1, $2, 'USER', $3, $4, $5, $6, $7, $8, 'HTTP', $9, $10, $11)`,
		auditID,
		action,
		actorUserID,
		tenantID,
		targetType,
		targetID,
		requestID,
		correlationID,
		nullableReason,
		encodedChanges,
		occurredAt,
	); err != nil {
		return fmt.Errorf("insert %s audit event: %w", action, err)
	}
	return nil
}

func writeContext(ctx context.Context) (actorUserID string, requestID string, correlationID string, err error) {
	actorUserID = requestcontext.UserID(ctx)
	requestID = requestcontext.RequestID(ctx)
	correlationID = requestcontext.CorrelationID(ctx)
	if !identifier.IsUUIDv7(actorUserID) ||
		!identifier.IsUUIDv7(requestID) ||
		!identifier.IsUUIDv7(correlationID) {
		return "", "", "", tenancy.ErrInvalidInput
	}
	return actorUserID, requestID, correlationID, nil
}

func tenantFingerprint(command tenancy.CreateTenantCommand) (string, error) {
	canonical, err := json.Marshal(struct {
		Name        string `json:"name"`
		OwnerUserID string `json:"owner_user_id"`
	}{Name: command.Name, OwnerUserID: command.OwnerUserID})
	if err != nil {
		return "", fmt.Errorf("marshal tenant request fingerprint: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func validName(value string) bool {
	length := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && length >= 1 && length <= 160
}

func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 255 {
		return false
	}
	for _, character := range []byte(value) {
		if character < '!' || character > '~' {
			return false
		}
	}
	return true
}

func uniqueRoleIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !identifier.IsUUIDv7(value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
