"use client";

import { useState, Suspense } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { signIn } from "next-auth/react";
import { ShieldCheck, Mail, Lock, LogIn, ArrowRight, ShieldAlert } from "lucide-react";
import { cn } from "@/lib/utils";

function LoginContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const callbackUrl = searchParams.get("callbackUrl") || "/dashboard";
  const registered = searchParams.get("registered");
  
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    try {
      const result = await signIn("credentials", {
        email,
        password,
        redirect: false,
      });

      if (result?.error) {
        setError("Invalid credentials. Access denied.");
      } else {
        router.push(callbackUrl);
        router.refresh();
      }
    } catch (err) {
      setError("An unexpected error occurred during authentication.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center p-6 relative overflow-hidden">
      {/* Background patterns */}
      <div className="absolute inset-0 z-0 pointer-events-none overflow-hidden">
        <div className="absolute top-[-20%] left-[-10%] w-[60%] h-[60%] bg-accent/10 blur-[150px] rounded-full" />
        <div className="absolute bottom-[-20%] right-[-10%] w-[60%] h-[60%] bg-blue/10 blur-[150px] rounded-full" />
      </div>

      <div className="w-full max-w-md space-y-8 animate-fade-up z-10">
        <div className="text-center space-y-3">
          <div className="inline-flex items-center gap-3 p-3 bg-accent/10 rounded-2xl border border-accent/20 mb-2">
            <ShieldCheck className="w-8 h-8 text-accent" />
          </div>
          <h1 className="text-3xl font-bold tracking-tight">Access the Perimeter</h1>
          <p className="text-muted-foreground text-sm max-w-xs mx-auto leading-relaxed">
            Verify your administrative identity to manage organizational security 
            and global access policies.
          </p>
        </div>

        <div className="bg-surface-1 border border-border rounded-2xl p-8 shadow-2xl relative overflow-hidden">
          {registered && !error && (
            <div className="mb-6 p-4 bg-green/10 border border-green/20 rounded-xl text-green text-[13px] font-medium flex items-center gap-3 animate-in fade-in slide-in-from-top-2">
              <ShieldCheck className="w-4 h-4" />
              Organization established successfully. Please sign in.
            </div>
          )}

          {error && (
            <div className="mb-6 p-4 bg-destructive/10 border border-destructive/20 rounded-xl text-destructive text-[13px] font-medium flex items-center gap-3 animate-in fade-in slide-in-from-top-2">
              <ShieldAlert className="w-4 h-4" />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-6">
            <div className="space-y-1.5 text-left">
              <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Identity Endpoint (Email)</label>
              <div className="relative group">
                <Mail className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                <input 
                  id="login-email"
                  type="email"
                  required
                  placeholder="admin@acme.com"
                  autoComplete="email"
                  className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-1.5 text-left">
              <div className="flex items-center justify-between ml-1">
                <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Secret Key (Password)</label>
                <Link href="#" className="text-[11px] font-bold text-accent hover:underline opacity-60 hover:opacity-100">Recovery</Link>
              </div>
              <div className="relative group">
                <Lock className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                <input 
                  id="login-password"
                  type="password"
                  required
                  placeholder="••••••••"
                  autoComplete="current-password"
                  className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
            </div>

            <button 
              type="submit" 
              disabled={loading}
              className="w-full btn btn-primary py-4 font-bold rounded-xl shadow-xl shadow-accent/20 gap-2 h-auto flex items-center justify-center transition-all group"
            >
              {loading ? (
                <div className="w-5 h-5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
              ) : (
                <>
                  <LogIn className="w-4 h-4" />
                  Sign in to Control Plane
                  <ArrowRight className="w-4 h-4 ml-1 group-hover:translate-x-1 transition-transform" />
                </>
              )}
            </button>
          </form>

          <p className="mt-8 text-center text-sm text-muted-foreground">
            New organization?{" "}
            <Link href="/register" className="text-accent font-bold hover:underline">
              Create perimeter
            </Link>
          </p>
        </div>

        <p className="text-center text-[10px] text-muted-foreground/60 uppercase tracking-widest font-mono">
          Stateless HMAC Authentication Enforced
        </p>
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-accent/20 border-t-accent rounded-full animate-spin" />
      </div>
    }>
      <LoginContent />
    </Suspense>
  );
}
