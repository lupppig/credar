import type { Metadata } from "next";

import { IdentityConsole } from "../identity-console";

export const metadata: Metadata = {
  title: "Identity console",
};

export default function ConsolePage() {
  return <IdentityConsole />;
}
