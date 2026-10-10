"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

type User = { id: string; email: string; status: string };
type MembershipSummary = {
  id: string;
  tenant: { id: string; name: string; status: string };
  status: string;
  role_names: string[];
};
type SessionContext = {
  user: User;
  memberships: MembershipSummary[];
  active_tenant_id: string | null;
};
type Permission = { name: string; description: string };
type Role = {
  id: string;
  name: string;
  kind: "SYSTEM" | "CUSTOM";
  permissions: Permission[];
};
type Member = { id: string; user: User; status: string; roles: Role[] };
type AuditEvent = {
  id: string;
  action: string;
  actor: { type: string; id: string | null };
  occurred_at: string;
  target_type: string;
};
type Problem = { code?: string; detail?: string };

const kratosBrowser =
  process.env.NEXT_PUBLIC_CREDAR_KRATOS_BROWSER_URL ?? "http://localhost:4433";

export function IdentityConsole({
  initialSession = null,
  initialMembers = [],
  initialRoles = [],
  initialAudit = [],
}: {
  initialSession?: SessionContext | null;
  initialMembers?: Member[];
  initialRoles?: Role[];
  initialAudit?: AuditEvent[];
} = {}) {
  const [session, setSession] = useState<SessionContext | null>(initialSession);
  const [tenantId, setTenantId] = useState(
    initialSession?.memberships[0]?.tenant.id ?? "",
  );
  const [members, setMembers] = useState<Member[]>(initialMembers);
  const [roles, setRoles] = useState<Role[]>(initialRoles);
  const [audit, setAudit] = useState<AuditEvent[]>(initialAudit);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [loading, setLoading] = useState(initialSession === null);
  const [memberUserId, setMemberUserId] = useState("");
  const [memberRoleId, setMemberRoleId] = useState("");
  const [roleName, setRoleName] = useState("");

  const loadSession = useCallback(async () => {
    try {
      const response = await fetch("/api/platform/session/context", {
        credentials: "include",
        cache: "no-store",
      });
      if (!response.ok) throw (await response.json()) as Problem;
      const value = (await response.json()) as SessionContext;
      setSession(value);
      setTenantId(
        (selected) => selected || value.memberships[0]?.tenant.id || "",
      );
      setProblem(null);
    } catch (error) {
      setProblem(error as Problem);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const task = window.setTimeout(() => void loadSession(), 0);
    return () => window.clearTimeout(task);
  }, [loadSession]);

  const loadTenant = useCallback(async (selectedTenantId: string) => {
    if (!selectedTenantId) return;
    const headers = { "X-Tenant-ID": selectedTenantId };
    await Promise.all([
      resource<{ data: Member[] }>("memberships", headers),
      resource<{ data: Role[] }>("roles", headers),
      resource<{ data: AuditEvent[] }>("audit-events", headers),
    ]).then(([memberPage, roleList, auditPage]) => {
      setMembers(memberPage.data);
      setRoles(roleList.data);
      setAudit(auditPage.data);
      setMemberRoleId((current) => current || roleList.data[0]?.id || "");
      setProblem(null);
    });
  }, []);

  useEffect(() => {
    if (!tenantId) return;
    const task = window.setTimeout(
      () =>
        void loadTenant(tenantId).catch((error: Problem) => setProblem(error)),
      0,
    );
    return () => window.clearTimeout(task);
  }, [loadTenant, tenantId]);

  const activeMembership = useMemo(
    () => session?.memberships.find((item) => item.tenant.id === tenantId),
    [session, tenantId],
  );
  const canManage = activeMembership?.role_names.some((name) =>
    ["TENANT_OWNER", "TENANT_ADMIN"].includes(name),
  );

  async function addMember(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await mutate("memberships", "POST", tenantId, {
        user_id: memberUserId,
        role_ids: [memberRoleId],
      });
      setMemberUserId("");
      await loadTenant(tenantId);
    } catch (error) {
      setProblem(error as Problem);
    }
  }

  async function createRole(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await mutate("roles", "POST", tenantId, {
        name: roleName,
        permission_names: ["tenants:read"],
      });
      setRoleName("");
      await loadTenant(tenantId);
    } catch (error) {
      setProblem(error as Problem);
    }
  }

  async function changeStatus(
    member: Member,
    status: "DISABLED" | "ACTIVE" | "REMOVED",
  ) {
    try {
      await mutate(`memberships/${member.id}`, "PATCH", tenantId, { status });
      await loadTenant(tenantId);
    } catch (error) {
      setProblem(error as Problem);
    }
  }

  if (loading) {
    return (
      <ConsoleState
        title="Resolving your workspace"
        detail="Checking the current Kratos session and tenant access."
      />
    );
  }
  if (!session) {
    return (
      <ConsoleState
        title="Sign in to Credar"
        detail={
          problem?.detail ??
          "Use your verified identity to enter an authorized workspace."
        }
      >
        <div className="action-row">
          <a
            className="primary-action"
            href={`${kratosBrowser}/self-service/login/browser`}
          >
            Sign in
          </a>
          <a
            className="secondary-action"
            href={`${kratosBrowser}/self-service/recovery/browser`}
          >
            Recover access
          </a>
          <a
            className="secondary-action"
            href={`${kratosBrowser}/self-service/verification/browser`}
          >
            Verify email
          </a>
        </div>
      </ConsoleState>
    );
  }

  return (
    <div className="console-shell">
      <aside className="console-sidebar">
        <p className="eyebrow">Workspace</p>
        <label htmlFor="tenant-switcher">Active tenant</label>
        <select
          id="tenant-switcher"
          value={tenantId}
          onChange={(event) => setTenantId(event.target.value)}
        >
          {session.memberships.map((membership) => (
            <option value={membership.tenant.id} key={membership.id}>
              {membership.tenant.name}
            </option>
          ))}
        </select>
        <nav aria-label="Administration sections">
          <a href="#team">Team</a>
          <a href="#roles">Roles</a>
          <a href="#permissions">Permission matrix</a>
          <a href="#audit">Audit history</a>
        </nav>
        <div className="identity-card">
          <strong>{session.user.email}</strong>
          <span>
            {activeMembership?.role_names.join(" · ") || "No assigned role"}
          </span>
        </div>
      </aside>
      <div className="console-content">
        <header className="console-heading">
          <div>
            <p className="eyebrow">Identity administration</p>
            <h1>Access is explicit.</h1>
          </div>
          <span className="status-chip">{members.length} members</span>
        </header>
        {problem && (
          <div className="denial-notice" role="alert">
            <strong>{problem.code ?? "request_denied"}</strong>
            <span>
              {problem.detail ?? "The operation was denied by server policy."}
            </span>
          </div>
        )}
        <section id="team" className="admin-panel">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Team</p>
              <h2>Membership lifecycle</h2>
            </div>
            <span className="status-chip">Tenant scoped</span>
          </div>
          <form className="inline-form" onSubmit={addMember}>
            <label>
              Existing user ID
              <input
                required
                value={memberUserId}
                onChange={(event) => setMemberUserId(event.target.value)}
                placeholder="UUIDv7"
                disabled={!canManage}
              />
            </label>
            <label>
              Initial role
              <select
                required
                value={memberRoleId}
                onChange={(event) => setMemberRoleId(event.target.value)}
                disabled={!canManage}
              >
                {roles.map((role) => (
                  <option value={role.id} key={role.id}>
                    {role.name}
                  </option>
                ))}
              </select>
            </label>
            <button
              className="primary-action compact"
              disabled={!canManage}
              title={
                canManage
                  ? "Add an existing platform user"
                  : "Requires memberships:manage and roles:manage"
              }
            >
              Add member
            </button>
          </form>
          <div className="data-list">
            {members.map((member) => (
              <article key={member.id}>
                <div>
                  <strong>{member.user.email}</strong>
                  <span>
                    {member.roles.map((role) => role.name).join(", ") ||
                      "No role"}
                  </span>
                </div>
                <div className="member-actions">
                  <span className={`state-tag ${member.status.toLowerCase()}`}>
                    {member.status}
                  </span>
                  {member.status !== "REMOVED" && (
                    <button
                      className="text-action"
                      disabled={!canManage}
                      title={
                        canManage
                          ? "Change membership access"
                          : "Requires memberships:manage"
                      }
                      onClick={() =>
                        void changeStatus(
                          member,
                          member.status === "ACTIVE" ? "DISABLED" : "ACTIVE",
                        )
                      }
                    >
                      {member.status === "ACTIVE" ? "Disable" : "Reactivate"}
                    </button>
                  )}
                  {member.status !== "REMOVED" && (
                    <button
                      className="text-action danger"
                      disabled={!canManage}
                      title={
                        canManage
                          ? "Remove membership permanently"
                          : "Requires memberships:manage"
                      }
                      onClick={() => void changeStatus(member, "REMOVED")}
                    >
                      Remove
                    </button>
                  )}
                </div>
              </article>
            ))}
          </div>
        </section>
        <section id="roles" className="admin-panel">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Roles</p>
              <h2>System and custom roles</h2>
            </div>
            <span className="status-chip">Closed catalogue</span>
          </div>
          <form className="inline-form" onSubmit={createRole}>
            <label>
              Custom role name
              <input
                required
                value={roleName}
                onChange={(event) => setRoleName(event.target.value)}
                placeholder="Credit analyst"
                disabled={!canManage}
              />
            </label>
            <p>
              Starts with <code>tenants:read</code>; expand it through the role
              API.
            </p>
            <button
              className="primary-action compact"
              disabled={!canManage}
              title={
                canManage
                  ? "Create a tenant-scoped custom role"
                  : "Requires roles:manage"
              }
            >
              Create role
            </button>
          </form>
          <div className="role-grid">
            {roles.map((role) => (
              <article key={role.id}>
                <div className="role-title">
                  <h3>{role.name}</h3>
                  <span>{role.kind}</span>
                </div>
                <p>{role.permissions.length} permissions</p>
                {role.kind === "SYSTEM" && (
                  <small>Immutable platform definition</small>
                )}
              </article>
            ))}
          </div>
        </section>
        <section id="permissions" className="admin-panel">
          <p className="eyebrow">Permission matrix</p>
          <h2>What each role can do</h2>
          <div className="permission-table" role="table">
            {roles.map((role) => (
              <div role="row" key={role.id}>
                <strong role="cell">{role.name}</strong>
                <span role="cell">
                  {role.permissions
                    .map((permission) => permission.name)
                    .join(" · ")}
                </span>
              </div>
            ))}
          </div>
        </section>
        <section id="audit" className="admin-panel">
          <p className="eyebrow">Audit</p>
          <h2>Immutable access history</h2>
          <div className="data-list">
            {audit.map((event) => (
              <article key={event.id}>
                <div>
                  <strong>{event.action}</strong>
                  <span>
                    {event.actor.type} · {event.target_type}
                  </span>
                </div>
                <time dateTime={event.occurred_at}>
                  {new Date(event.occurred_at).toLocaleString()}
                </time>
              </article>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}

function ConsoleState({
  title,
  detail,
  children,
}: {
  title: string;
  detail: string;
  children?: React.ReactNode;
}) {
  return (
    <section className="state-panel">
      <p className="eyebrow">Credar identity</p>
      <h1>{title}</h1>
      <p>{detail}</p>
      {children}
    </section>
  );
}

async function resource<T>(path: string, headers: HeadersInit): Promise<T> {
  const response = await fetch(`/api/platform/${path}`, {
    headers,
    credentials: "include",
    cache: "no-store",
  });
  if (!response.ok) throw (await response.json()) as Problem;
  return (await response.json()) as T;
}

async function mutate(
  path: string,
  method: string,
  tenantId: string,
  body: unknown,
) {
  const response = await fetch(`/api/platform/${path}`, {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json", "X-Tenant-ID": tenantId },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw (await response.json()) as Problem;
  return response.status === 204 ? undefined : response.json();
}
