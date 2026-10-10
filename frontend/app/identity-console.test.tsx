import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { IdentityConsole } from "./identity-console";

describe("identity administration console", () => {
  it("explains the first-run state before a workspace is assigned", () => {
    const markup = renderToStaticMarkup(
      <IdentityConsole
        initialSession={{
          user: { id: "user", email: "new@example.com", status: "ACTIVE" },
          active_tenant_id: null,
          memberships: [],
        }}
      />,
    );

    expect(markup).toContain("Your identity is ready.");
    expect(markup).toContain("no workspace has been assigned yet");
    expect(markup).toContain('href="/auth/settings"');
    expect(markup).toContain('href="/auth/logout"');
  });

  it("renders tenant switching, administration views, and explained denials", () => {
    const markup = renderToStaticMarkup(
      <IdentityConsole
        initialSession={{
          user: { id: "user", email: "member@example.com", status: "ACTIVE" },
          active_tenant_id: null,
          memberships: [
            {
              id: "membership",
              status: "ACTIVE",
              tenant: { id: "tenant-a", name: "Alpha", status: "ACTIVE" },
              role_names: ["TENANT_MEMBER"],
            },
            {
              id: "membership-b",
              status: "ACTIVE",
              tenant: { id: "tenant-b", name: "Beta", status: "ACTIVE" },
              role_names: ["TENANT_AUDITOR"],
            },
          ],
        }}
        initialRoles={[
          {
            id: "role",
            name: "TENANT_MEMBER",
            kind: "SYSTEM",
            permissions: [
              { name: "tenants:read", description: "Read the tenant" },
            ],
          },
        ]}
      />,
    );

    expect(markup).toContain("Alpha");
    expect(markup).toContain("Beta");
    expect(markup).toContain("Permission matrix");
    expect(markup).toContain("Immutable platform definition");
    expect(markup).toContain("Requires memberships:manage and roles:manage");
    expect(markup).toContain("Requires roles:manage");
  });
});
