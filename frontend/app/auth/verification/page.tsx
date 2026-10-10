import type { Metadata } from "next";

import { AuthFlow } from "../auth-flow";

export const metadata: Metadata = { title: "Verify email" };

export default async function VerificationPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;
  return <AuthFlow kind="verification" flowId={flow} />;
}
