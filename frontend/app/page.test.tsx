import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import Home from "./page";

describe("enterprise shell home", () => {
  it("starts the authenticated workspace journey", () => {
    const markup = renderToStaticMarkup(<Home />);

    expect(markup).toContain("Resolving your workspace");
    expect(markup).toContain("Checking the current Kratos session");
  });
});
