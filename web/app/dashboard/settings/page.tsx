"use client";

import { useState } from "react";
import Shell from "@/components/layout/Shell";
import { 
  ShieldCheck, 
  Key, 
  UserCircle2, 
  Smartphone, 
  Trash2, 
  ShieldAlert,
  Fingerprint,
  RefreshCw,
  LogOut,
  MapPin,
  Laptop
} from "lucide-react";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const [mfaEnabled, setMfaEnabled] = useState(false);

  return (
    <Shell 
      title="Organization Security Settings" 
      crumbs={["Governance", "Settings"]}
    >
      <div className="grid grid-cols-12 gap-8 animate-fade-up">
        {/* Left Column - Navigation */}
        <div className="col-span-3 space-y-2">
          <SettingsNavLink active icon={ShieldCheck}>Security & Compliance</SettingsNavLink>
          <SettingsNavLink icon={UserCircle2}>Identity & Access</SettingsNavLink>
          <SettingsNavLink icon={Key}>API Credentials</SettingsNavLink>
          <SettingsNavLink icon={RefreshCw}>Audit Retention</SettingsNavLink>
        </div>

        {/* Right Column - Content */}
        <div className="col-span-9 space-y-10">
          {/* Multi-Factor Authentication Section */}
          <section className="space-y-6">
            <div className="space-y-1">
              <h3 className="text-lg font-bold tracking-tight">Multi-Factor Authentication (MFA)</h3>
              <p className="text-xs text-muted-foreground leading-relaxed">
                Add an extra layer of security to your organization admin account. 
                Supported methods include TOTP (Authenticator App) and WebAuthn (YubiKey/TouchID).
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <MfaMethodCard 
                title="Authenticator App" 
                desc="Generate one-time codes via Google Authenticator or 1Password."
                icon={Smartphone}
                status={mfaEnabled ? "enabled" : "disabled"}
                onToggle={() => setMfaEnabled(!mfaEnabled)}
              />
              <MfaMethodCard 
                title="Security Key (WebAuthn)" 
                desc="Use a physical hardware key or biometric challenge for high-trust auth."
                icon={Fingerprint}
                status="unsupported"
                disabled
              />
            </div>
          </section>

          {/* Active Sessions Section */}
          <section className="space-y-6">
            <div className="flex items-center justify-between">
              <div className="space-y-1">
                <h3 className="text-lg font-bold tracking-tight text-[16px]">Active Management Sessions</h3>
                <p className="text-xs text-muted-foreground leading-relaxed">
                  These are the browsers and devices that can currently manage your 
                  organization. Sessions are enforced via global JWT rotation.
                </p>
              </div>
              <button className="btn btn-ghost text-[11px] h-9 px-4 border-destructive/20 text-destructive hover:bg-destructive/5">
                Revoke all other sessions
              </button>
            </div>

            <div className="bg-surface-1 border border-border rounded-xl divide-y divide-border overflow-hidden">
              <SessionItem 
                device="macOS • Chrome 122.0.0" 
                ip="192.168.1.45" 
                location="San Francisco, US" 
                current 
              />
              <SessionItem 
                device="Ubuntu • Firefox 115.0.0" 
                ip="172.16.2.11" 
                location="London, UK" 
                time="Active 12m ago" 
              />
              <SessionItem 
                device="iOS • Safari Mobile" 
                ip="10.0.0.82" 
                location="Paris, FR" 
                time="Active 2h ago" 
              />
            </div>
          </section>

          {/* Danger Zone */}
          <section className="p-6 border border-destructive/20 bg-destructive/5 rounded-xl space-y-4">
            <div className="flex items-center gap-3">
              <ShieldAlert className="w-5 h-5 text-destructive" />
              <h3 className="text-md font-bold tracking-tight text-destructive">Advanced Risk Mitigation</h3>
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">
              If you suspect an active compromise, you can immediately suspend the entire 
              control plane. This will block all programmatic access while preserving 
              the cryptographic integrity of the audit logs.
            </p>
            <button className="btn btn-primary bg-destructive hover:bg-red-600 text-[12px] font-bold h-10 px-6 border-none">
              Panic Mode: Terminate All Access
            </button>
          </section>
        </div>
      </div>
    </Shell>
  );
}

function SettingsNavLink({ children, active, icon: Icon }: any) {
  return (
    <button className={cn(
      "w-full flex items-center gap-3 px-4 py-2.5 rounded-lg text-[13px] font-medium transition-all text-left",
      active ? "bg-accent/10 text-accent font-bold" : "text-muted-foreground hover:bg-secondary hover:text-foreground"
    )}>
      <Icon className={cn("w-4 h-4", active ? "text-accent" : "text-muted-foreground")} />
      {children}
    </button>
  );
}

function MfaMethodCard({ title, desc, icon: Icon, status, onToggle, disabled }: any) {
  return (
    <div className={cn(
      "p-5 bg-surface-1 border border-border rounded-xl space-y-4 transition-all group",
      status === "enabled" ? "border-green/20" : "hover:border-accent/40 cursor-pointer",
      disabled && "opacity-50 grayscale cursor-not-allowed"
    )}>
      <div className="flex items-center justify-between">
        <div className="w-10 h-10 rounded-xl bg-secondary grid place-items-center group-hover:scale-110 transition-transform">
          <Icon className="w-5 h-5 text-muted-foreground group-hover:text-foreground" />
        </div>
        <span className={cn(
          "tag uppercase text-[9px]",
          status === "enabled" ? "tag-green" : status === "disabled" ? "tag-amber" : "tag-blue"
        )}>
          {status}
        </span>
      </div>
      <div className="space-y-1">
        <h4 className="text-[13px] font-bold tracking-tight">{title}</h4>
        <p className="text-[11px] text-muted-foreground leading-relaxed">{desc}</p>
      </div>
      {!disabled && (
        <button 
          onClick={onToggle}
          className={cn(
            "w-full btn py-2 text-[11px] font-bold",
            status === "enabled" ? "btn-ghost border-destructive/20 text-destructive hover:bg-destructive/5" : "btn-primary"
          )}
        >
          {status === "enabled" ? "Revoke Access" : "Enroll Member"}
        </button>
      )}
    </div>
  );
}

function SessionItem({ device, ip, location, current, time }: any) {
  return (
    <div className="p-4 flex items-center justify-between group">
      <div className="flex items-center gap-4">
        <div className="w-10 h-10 rounded-xl bg-secondary grid place-items-center shrink-0 group-hover:bg-accent/5 transition-colors">
          <Laptop className={cn("w-5 h-5 transition-colors", current ? "text-accent" : "text-muted-foreground group-hover:text-foreground")} />
        </div>
        <div className="space-y-0.5 text-left">
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-bold tracking-tight underline decoration-accent/20 underline-offset-4 decoration-dashed">{device}</span>
            {current && <span className="tag tag-blue text-[8px] px-1.5 py-0">Current</span>}
          </div>
          <div className="flex items-center gap-3 text-[11px] text-muted-foreground tabular-nums">
            <span className="flex items-center gap-1"><MapPin className="w-3 h-3" />{location}</span>
            <span>•</span>
            <span>{ip}</span>
            {time && <span>•</span>}
            {time && <span>{time}</span>}
          </div>
        </div>
      </div>
      {!current && (
        <button className="p-2 hover:bg-destructive/10 rounded-lg transition-colors text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100">
          <LogOut className="w-4 h-4" />
        </button>
      )}
    </div>
  );
}
