import type { Metadata } from "next";

import { AuthError } from "./error-client";

export const metadata: Metadata = { title: "Identity error" };

export default async function AuthErrorPage({
  searchParams,
}: {
  searchParams: Promise<{ id?: string }>;
}) {
  const { id } = await searchParams;
  return <AuthError errorId={id} />;
}
