import type { Metadata } from "next";

import { AuthFlow } from "../auth-flow";

export const metadata: Metadata = { title: "Recover access" };

export default async function RecoveryPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;
  return <AuthFlow kind="recovery" flowId={flow} />;
}
