import type { Metadata } from "next";

import { LogoutClient } from "./logout-client";

export const metadata: Metadata = { title: "Sign out" };

export default function LogoutPage() {
  return <LogoutClient />;
}
