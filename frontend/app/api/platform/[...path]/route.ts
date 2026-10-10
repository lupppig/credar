import type { NextRequest } from "next/server";

import { loadServerEnvironment } from "../../../../lib/environment";

type RouteContext = { params: Promise<{ path: string[] }> };

async function proxy(request: NextRequest, context: RouteContext) {
  const { path } = await context.params;
  const incomingUrl = new URL(request.url);
  const target = new URL(
    `/v1/${path.map(encodeURIComponent).join("/")}${incomingUrl.search}`,
    loadServerEnvironment().backendInternalUrl,
  );
  const headers = new Headers({ Accept: "application/json" });
  for (const name of [
    "content-type",
    "cookie",
    "idempotency-key",
    "x-correlation-id",
    "x-session-token",
    "x-tenant-id",
  ]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  const hasBody = !["GET", "HEAD"].includes(request.method);
  const response = await fetch(target, {
    method: request.method,
    headers,
    body: hasBody ? await request.arrayBuffer() : undefined,
    cache: "no-store",
  });
  const outgoing = new Headers();
  for (const name of ["content-type", "x-correlation-id", "x-request-id"]) {
    const value = response.headers.get(name);
    if (value) outgoing.set(name, value);
  }
  return new Response(response.body, {
    status: response.status,
    headers: outgoing,
  });
}

export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
export const PUT = proxy;
export const DELETE = proxy;
