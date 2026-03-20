import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "OpenGuard — Security Dashboard",
  description:
    "Enterprise security platform — IAM, policy engine, threat detection, and compliance reporting.",
  icons: { icon: "/favicon.ico" },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
