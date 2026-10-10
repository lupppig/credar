import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import Home from "./page";

describe("Credar platform landing page", () => {
  it("presents the credit and payment narrative and a path into the product", () => {
    const markup = renderToStaticMarkup(<Home />);

    expect(markup).toContain("Launch credit.");
    expect(markup).toContain("Move payments.");
    expect(markup).toContain("Fund possibility.");
    expect(markup).toContain("From the first yes to the final payment.");
    expect(markup).toContain("Embedded credit");
    expect(markup).toContain("Buy now, pay later");
    expect(markup).toContain('href="/auth/registration"');
    expect(markup).not.toContain("PostgreSQL");
    expect(markup).not.toContain("JetStream");
    expect(markup).not.toContain("Redis");
  });
});
