import type { Metadata } from "next";

import { AuthFlow } from "../auth-flow";

export const metadata: Metadata = { title: "Sign in" };

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;
  return <AuthFlow kind="login" flowId={flow} />;
}
