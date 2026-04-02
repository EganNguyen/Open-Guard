"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ShieldCheck, ArrowRight, Building, Mail, Lock, CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/utils";

export default function RegisterPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [form, setForm] = useState({
    org_name: "",
    email: "",
    password: "",
    confirm_password: "",
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (form.password !== form.confirm_password) {
      setError("Passwords do not match");
      return;
    }

    setLoading(true);
    setError("");

    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/v1/auth/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          org_name: form.org_name,
          email: form.email,
          password: form.password,
        }),
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error?.message || "Registration failed");
      }

      // After successful registration, redirect to login
      router.push("/login?registered=true");
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-6 relative overflow-hidden">
      {/* Decorative Glows */}
      <div className="absolute top-0 left-0 w-full h-full pointer-events-none opacity-20">
        <div className="absolute top-[-10%] left-[-10%] w-[40%] h-[40%] bg-accent/20 blur-[120px] rounded-full" />
        <div className="absolute bottom-[-10%] right-[-10%] w-[40%] h-[40%] bg-blue/10 blur-[120px] rounded-full" />
      </div>

      <div className="w-full max-w-md space-y-8 animate-fade-up relative z-10">
        <div className="text-center space-y-2">
          <div className="inline-flex items-center gap-2.5 mb-2">
            <div className="p-2 rounded-lg bg-accent/10 border border-accent/20">
              <ShieldCheck className="w-6 h-6 text-accent" />
            </div>
            <span className="text-xl font-bold tracking-tight">OpenGuard</span>
          </div>
          <h1 className="text-3xl font-bold tracking-tight">Establish your perimeter</h1>
          <p className="text-muted-foreground text-sm">
            Create an organization to begin centralized identity management and 
            governance across your infrastructure.
          </p>
        </div>

        <div className="bg-surface-1 border border-border rounded-2xl p-8 shadow-2xl relative overflow-hidden">
          {error && (
            <div className="mb-6 p-4 bg-destructive/10 border border-destructive/20 rounded-xl text-destructive text-[13px] font-medium flex items-center gap-3 animate-in fade-in slide-in-from-top-2">
              <ShieldCheck className="w-4 h-4 rotate-180" />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="space-y-1.5 text-left">
              <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Organization Name</label>
              <div className="relative group">
                <Building className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                <input 
                  id="reg-org"
                  required
                  placeholder="Acme Global Inc."
                  className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                  value={form.org_name}
                  onChange={(e) => setForm({ ...form, org_name: e.target.value })}
                />
              </div>
            </div>

            <div className="space-y-1.5 text-left">
              <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Administrator Email</label>
              <div className="relative group">
                <Mail className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                <input 
                  id="reg-email"
                  type="email"
                  required
                  placeholder="admin@acme.com"
                  className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5 text-left">
                <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Password</label>
                <div className="relative group">
                  <Lock className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                  <input 
                    id="reg-password"
                    type="password"
                    required
                    placeholder="••••••••"
                    className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                    value={form.password}
                    onChange={(e) => setForm({ ...form, password: e.target.value })}
                  />
                </div>
              </div>
              <div className="space-y-1.5 text-left">
                <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Confirm</label>
                <div className="relative group">
                  <CheckCircle2 className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground group-focus-within:text-accent transition-colors" />
                  <input 
                    id="reg-confirm"
                    type="password"
                    required
                    placeholder="••••••••"
                    className="w-full bg-secondary/50 border border-border rounded-xl pl-11 pr-4 py-3 text-[14px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent transition-all"
                    value={form.confirm_password}
                    onChange={(e) => setForm({ ...form, confirm_password: e.target.value })}
                  />
                </div>
              </div>
            </div>

            <div className="pt-2">
              <button 
                type="submit" 
                disabled={loading}
                className="w-full btn btn-primary py-4 font-bold rounded-xl shadow-xl shadow-accent/20 gap-2 h-auto"
              >
                {loading ? "Establishing Perimeter..." : "Register Organization"}
                <ArrowRight className="w-4 h-4" />
              </button>
            </div>
          </form>

          <p className="mt-8 text-center text-sm text-muted-foreground">
            Already have an organization?{" "}
            <Link href="/login" className="text-accent font-bold hover:underline">
              Sign in
            </Link>
          </p>
        </div>

        <div className="text-center">
          <p className="text-[11px] text-muted-foreground uppercase tracking-widest font-medium opacity-60">
            Enterprise Identity Management — Phase 1 Foundation
          </p>
        </div>
      </div>
    </div>
  );
}
