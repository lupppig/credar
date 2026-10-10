"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

const kratosBrowser =
  process.env.NEXT_PUBLIC_CREDAR_KRATOS_BROWSER_URL ?? "http://localhost:4433";

export function LogoutClient() {
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const destination = new URL("/self-service/logout/browser", kratosBrowser);
    destination.searchParams.set("return_to", `${window.location.origin}/`);

    const controller = new AbortController();
    fetch(destination, {
      credentials: "include",
      headers: { Accept: "application/json" },
      cache: "no-store",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) throw new Error("The session could not be closed.");
        return (await response.json()) as { logout_url?: string };
      })
      .then(({ logout_url: logoutUrl }) => {
        if (!logoutUrl) throw new Error("The logout response was incomplete.");
        window.location.replace(logoutUrl);
      })
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === "AbortError")
          return;
        setError(
          reason instanceof Error ? reason.message : "Unable to sign out.",
        );
      });

    return () => controller.abort();
  }, []);

  return (
    <section className="state-panel">
      <div className="state-indicator" aria-hidden="true" />
      <p className="eyebrow">Secure session</p>
      <h1>Signing you out.</h1>
      <p>
        {error ??
          "Your Credar session is being closed with the identity service."}
      </p>
      {error && (
        <div className="action-row">
          <Link className="primary-action" href="/console">
            Return to console
          </Link>
          <Link className="secondary-action" href="/">
            Return home
          </Link>
        </div>
      )}
    </section>
  );
}
