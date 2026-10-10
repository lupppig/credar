import type { Metadata } from "next";

import { AuthFlow } from "../auth-flow";

export const metadata: Metadata = { title: "Create account" };

export default async function RegistrationPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;
  return <AuthFlow kind="registration" flowId={flow} />;
}
