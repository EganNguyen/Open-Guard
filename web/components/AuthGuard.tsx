"use client";

import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null);
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    const token = localStorage.getItem("access_token");
    if (!token) {
      setIsAuthenticated(false);
      router.push(`/login?callbackUrl=${encodeURIComponent(pathname)}`);
    } else {
      setIsAuthenticated(true);
    }
  }, [router, pathname]);

  if (isAuthenticated === null) {
    return (
      <div style={{ 
        height: "100vh", 
        display: "flex", 
        alignItems: "center", 
        justifyContent: "center",
        background: "var(--bg)",
        color: "var(--muted)",
        fontFamily: "var(--font-sans)"
      }}>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "16px" }}>
          <div className="spinner" style={{ 
            width: "24px", 
            height: "24px", 
            border: "2px solid var(--border)", 
            borderTopColor: "var(--accent)", 
            borderRadius: "50%",
            animation: "spin 0.8s linear infinite"
          }} />
          <span style={{ fontSize: "14px" }}>Verifying session...</span>
        </div>
        <style jsx>{`
          @keyframes spin {
            to { transform: rotate(360deg); }
          }
        `}</style>
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
