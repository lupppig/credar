import type { Metadata } from "next";

import { AuthFlow } from "../auth-flow";

export const metadata: Metadata = { title: "Account settings" };

export default async function SettingsPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;
  return <AuthFlow kind="settings" flowId={flow} />;
}
