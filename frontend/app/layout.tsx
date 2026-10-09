import type { Metadata } from "next";
import type { ReactNode } from "react";

import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Credar Console",
    template: "%s · Credar",
  },
  description: "Enterprise operations for multi-tenant credit infrastructure",
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
            <div className="brand" aria-label="Credar">
              <span className="brand-mark" aria-hidden="true">
                MR
              </span>
              <span>
                <strong>Credar</strong>
                <small>Enterprise console</small>
              </span>
            </div>
            <p className="environment-label">
              <span aria-hidden="true" /> Foundation build
            </p>
          </header>
          <main id="main-content" className="app-content">
            {children}
          </main>
          <footer className="app-footer">
            <span>Credit infrastructure</span>
            <span>Authentication and workspace navigation arrive next.</span>
          </footer>
        </div>
      </body>
    </html>
  );
}
