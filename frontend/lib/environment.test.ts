import { describe, expect, it } from "vitest";

import { loadPublicEnvironment, loadServerEnvironment } from "./environment";

describe("frontend environment", () => {
  it("provides safe local defaults", () => {
    expect(loadPublicEnvironment({}).apiBaseUrl).toBe("http://localhost:8080");
    expect(loadServerEnvironment({}).backendInternalUrl).toBe(
      "http://127.0.0.1:8080",
    );
  });

  it("normalizes validated explicit URLs", () => {
    expect(
      loadPublicEnvironment({
        NEXT_PUBLIC_CREDAR_API_BASE_URL: "https://api.example.test/",
      }).apiBaseUrl,
    ).toBe("https://api.example.test");
    expect(
      loadServerEnvironment({
        CREDAR_BACKEND_INTERNAL_URL: "http://backend:8080",
      }).backendInternalUrl,
    ).toBe("http://backend:8080");
  });

  it.each([
    ["relative URL", "api/v1"],
    ["unsupported scheme", "ftp://api.example.test"],
    ["embedded credentials", "https://user:secret@api.example.test"],
    ["unexpected path", "https://api.example.test/v1"],
  ])("rejects %s", (_description, value) => {
    expect(() =>
      loadServerEnvironment({ CREDAR_BACKEND_INTERNAL_URL: value }),
    ).toThrow("CREDAR_BACKEND_INTERNAL_URL");
  });
});
