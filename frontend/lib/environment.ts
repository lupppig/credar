export type PublicEnvironment = Readonly<{
  apiBaseUrl: string;
}>;

export type ServerEnvironment = Readonly<{
  backendInternalUrl: string;
}>;

type EnvironmentSource = Readonly<Record<string, string | undefined>>;

const localPublicApiBaseUrl = "http://localhost:8080";
const localBackendInternalUrl = "http://127.0.0.1:8080";

export function loadPublicEnvironment(
  source: EnvironmentSource = {
    NEXT_PUBLIC_CREDAR_API_BASE_URL:
      process.env.NEXT_PUBLIC_CREDAR_API_BASE_URL,
  },
): PublicEnvironment {
  return {
    apiBaseUrl: absoluteHttpUrl(
      "NEXT_PUBLIC_CREDAR_API_BASE_URL",
      source.NEXT_PUBLIC_CREDAR_API_BASE_URL ?? localPublicApiBaseUrl,
    ),
  };
}

export function loadServerEnvironment(
  source: EnvironmentSource = process.env,
): ServerEnvironment {
  return {
    backendInternalUrl: absoluteHttpUrl(
      "CREDAR_BACKEND_INTERNAL_URL",
      source.CREDAR_BACKEND_INTERNAL_URL ?? localBackendInternalUrl,
    ),
  };
}

function absoluteHttpUrl(name: string, value: string): string {
  if (value.trim() !== value || value === "") {
    throw new Error(`${name} must be a non-empty URL without whitespace`);
  }

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`${name} must be an absolute HTTP or HTTPS URL`);
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error(`${name} must use HTTP or HTTPS`);
  }
  if (
    parsed.username !== "" ||
    parsed.password !== "" ||
    (parsed.pathname !== "" && parsed.pathname !== "/") ||
    parsed.search !== "" ||
    parsed.hash !== ""
  ) {
    throw new Error(
      `${name} must not contain credentials, a path, query, or fragment`,
    );
  }

  return parsed.origin;
}
