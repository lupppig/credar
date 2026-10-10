"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

const kratosBrowser =
  process.env.NEXT_PUBLIC_CREDAR_KRATOS_BROWSER_URL ?? "http://localhost:4433";

export function AuthError({ errorId }: { errorId?: string }) {
  const [message, setMessage] = useState(
    "The identity flow could not be completed.",
  );

  useEffect(() => {
    if (!errorId) return;
    fetch(
      `${kratosBrowser}/self-service/errors?id=${encodeURIComponent(errorId)}`,
      {
        credentials: "include",
        cache: "no-store",
      },
    )
      .then(async (response) => {
        if (!response.ok) throw new Error();
        return (await response.json()) as { error?: { message?: string } };
      })
      .then((payload) => {
        if (payload.error?.message) setMessage(payload.error.message);
      })
      .catch(() => undefined);
  }, [errorId]);

  return (
    <section className="state-panel">
      <p className="eyebrow">Identity service</p>
      <h1>We could not verify that request.</h1>
      <p>{message}</p>
      <div className="action-row">
        <a className="primary-action" href="/auth/login">
          Start again
        </a>
        <Link className="secondary-action" href="/">
          Return home
        </Link>
      </div>
    </section>
  );
}
