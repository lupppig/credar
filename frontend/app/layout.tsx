import type { Metadata } from "next";
import Link from "next/link";
import type { ReactNode } from "react";

import { BrandLogo } from "./brand-logo";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Credar | Credit and Payment Infrastructure for Businesses",
    template: "%s · Credar",
  },
  description:
    "Launch and operate embedded credit, BNPL, merchant financing, payments, and repayment products on one enterprise platform.",
  keywords: [
    "credit infrastructure",
    "payment infrastructure",
    "embedded credit",
    "BNPL platform",
    "lending infrastructure",
    "repayment collection",
    "merchant financing",
    "B2B credit platform",
  ],
  openGraph: {
    title: "Credar: Credit moves possibility forward",
    description:
      "Enterprise infrastructure for building credit and payment products that move businesses forward.",
    type: "website",
    siteName: "Credar",
  },
  twitter: {
    card: "summary_large_image",
    title: "Credar: Credit moves possibility forward",
    description:
      "Enterprise credit and payment infrastructure for modern businesses.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <div className="app-frame">
          <header className="app-header">
            <Link className="brand" href="/" aria-label="Credar home">
              <BrandLogo />
              <span>
                <strong>Credar</strong>
                <small>Credit moves forward</small>
              </span>
            </Link>
            <nav className="site-nav" aria-label="Primary navigation">
              <Link href="/#products">Products</Link>
              <Link href="/#use-cases">Use cases</Link>
              <Link href="/#why-credar">Why Credar</Link>
              <a href="/auth/login">Sign in</a>
              <Link className="nav-action" href="/console">
                Open console
              </Link>
            </nav>
          </header>
          <main id="main-content" className="app-content">
            {children}
          </main>
          <footer className="app-footer">
            <span>Credar · Credit moves possibility forward.</span>
            <span>Credit · Payments · Enterprise operations</span>
          </footer>
        </div>
      </body>
    </html>
  );
}
