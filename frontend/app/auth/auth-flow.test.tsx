import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AuthFlow } from "./auth-flow";

describe("Kratos self-service shell", () => {
  it("renders the login context while a browser flow is being resolved", () => {
    const markup = renderToStaticMarkup(<AuthFlow kind="login" />);

    expect(markup).toContain("Welcome back.");
    expect(markup).toContain("Connecting to the identity service");
    expect(markup).toContain('href="/auth/recovery"');
    expect(markup).toContain('href="/auth/registration"');
  });

  it("renders registration-specific copy", () => {
    const markup = renderToStaticMarkup(<AuthFlow kind="registration" />);

    expect(markup).toContain("Start with a secure foundation.");
    expect(markup).toContain("Already registered?");
  });
});
