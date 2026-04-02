"use client";

import Shell from "@/components/layout/Shell";
import { 
  Users, 
  ShieldCheck, 
  Activity, 
  History, 
  ChevronRight, 
  Plus, 
  Settings, 
  AlertTriangle 
} from "lucide-react";
import { cn } from "@/lib/utils";

export default function DashboardPage() {
  return (
    <Shell 
      title="Security Overview" 
      crumbs={["Dashboard"]}
    >
      <div className="grid grid-cols-4 gap-6">
        <StatCard 
          title="Active Alerts" 
          value="3" 
          trend="+2" 
          status="destructive" 
          icon={AlertTriangle} 
        />
        <StatCard 
          title="Managed Users" 
          value="124" 
          trend="+5" 
          status="info" 
          icon={Users} 
        />
        <StatCard 
          title="Audit Volume (24h)" 
          value="1.2k" 
          trend="-12%" 
          status="success" 
          icon={History} 
        />
        <StatCard 
          title="Active Policies" 
          value="8" 
          trend="—" 
          status="primary" 
          icon={ShieldCheck} 
        />
      </div>

      <div className="grid grid-cols-3 gap-8 mt-10">
        <div className="col-span-2 space-y-8">
          {/* Quick Actions */}
          <section className="bg-surface-1 border border-border rounded-xl p-8 space-y-6 relative overflow-hidden">
            <div className="absolute top-0 right-0 w-32 h-32 bg-accent/5 rounded-full -translate-y-16 translate-x-16" />
            <div className="space-y-2">
              <span className="text-[11px] font-bold uppercase tracking-wider text-accent">Getting Started</span>
              <h2 className="text-xl font-bold tracking-tight">Verify your organization domain</h2>
              <p className="text-sm text-muted-foreground max-w-md">
                Claim your domain to automatically manage user accounts and enforce 
                security policies across all organization-owned identities.
              </p>
            </div>
            
            <div className="flex items-center gap-4">
              <button className="btn btn-primary h-10 px-6 gap-2">
                Verify domain
                <ChevronRight className="w-3.5 h-3.5" />
              </button>
              <button className="btn btn-ghost h-10 px-6">View documentation</button>
            </div>
            
            <div className="flex items-center gap-4 pt-4 border-t border-border mt-6">
              <div className="flex -space-x-2">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="w-8 h-8 rounded-full border-2 border-surface-1 bg-secondary grid place-items-center">
                    <Users className="w-3.5 h-3.5 text-muted-foreground" />
                  </div>
                ))}
              </div>
              <span className="text-[12px] text-muted-foreground font-medium">8 users pending domain verification</span>
            </div>
          </section>

          {/* Activity Section */}
          <section className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="font-bold tracking-tight">Recent Activity</h3>
              <button className="text-[12px] text-accent font-medium hover:underline">View all trail</button>
            </div>
            <div className="bg-surface-1 border border-border rounded-xl divide-y divide-border">
              <ActivityItem 
                title="SAML SSO configured" 
                desc="Azure AD integration was successfully tested by Jane D." 
                time="2m ago" 
                icon={Settings} 
              />
              <ActivityItem 
                title="New policy version created" 
                desc="Production RBAC policy v2.4 (Strict Isolation) by Bob K." 
                time="15m ago" 
                icon={ShieldCheck} 
              />
              <ActivityItem 
                title="Outbox relay delay" 
                desc="Infrastructure latency detected in us-east-1 queue" 
                time="42m ago" 
                icon={Activity} 
                status="warning"
              />
            </div>
          </section>
        </div>

        <aside className="space-y-8">
          {/* Recommendations Panel */}
          <div className="bg-card border border-border rounded-xl p-6 space-y-6">
            <div className="flex items-center justify-between">
              <h3 className="font-bold tracking-tight">Security Posture</h3>
              <div className="w-8 h-8 rounded-full border-2 border-accent/20 flex items-center justify-center text-[11px] font-bold text-accent">
                84%
              </div>
            </div>
            
            <div className="space-y-4">
              <RecommendationItem 
                title="Enable MFA enforcement" 
                desc="Require two-factor for all admin accounts" 
                priority="high"
              />
              <RecommendationItem 
                title="Rotate SCIM tokens" 
                desc="Tokens for Okta have been active for >90 days" 
                priority="med"
              />
              <RecommendationItem 
                title="Audit log retention" 
                desc="Current retention set to minimum (30 days)" 
                priority="low"
              />
            </div>

            <button className="w-full btn btn-ghost py-2 mt-4 text-[12px] gap-2 border-accent/20 hover:border-accent">
              <Settings className="w-3.5 h-3.5" />
              Manage Security Policies
            </button>
          </div>

          {/* Quick Stats Panel */}
          <div className="bg-surface-1 border border-border rounded-xl p-6 space-y-4">
            <h3 className="text-sm font-bold tracking-tight flex items-center gap-2">
              <Activity className="w-4 h-4 text-accent" />
              SLO Status
            </h3>
            <div className="space-y-3">
              <SloProgress name="Login Latency" value="88ms" percent={65} />
              <SloProgress name="Policy Evaluation" value="22ms" percent={82} />
              <SloProgress name="Audit Ingestion" value="4ms" percent={95} />
            </div>
          </div>
        </aside>
      </div>
    </Shell>
  );
}

function StatCard({ title, value, trend, status, icon: Icon }: any) {
  return (
    <div className="p-6 bg-surface-1 border border-border rounded-xl space-y-3 relative overflow-hidden transition-all hover:border-accent/40 hover:shadow-lg hover:shadow-accent/5 cursor-default group">
      <div className="flex items-center justify-between relative z-10">
        <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{title}</span>
        <div className={cn(
          "w-8 h-8 rounded-lg grid place-items-center transition-colors shadow-sm",
          status === "destructive" ? "bg-destructive/10 text-destructive group-hover:bg-destructive/20" :
          status === "success" ? "bg-green/10 text-green group-hover:bg-green/20" :
          "bg-accent/10 text-accent group-hover:bg-accent/20"
        )}>
          <Icon className="w-4 h-4" />
        </div>
      </div>
      <div className="flex items-end gap-3 relative z-10">
        <div className={cn(
          "text-3xl font-bold tracking-tight tabular-nums",
          status === "destructive" && "text-destructive"
        )}>{value}</div>
        <div className={cn(
          "text-[12px] font-medium mb-1 flex items-center gap-1",
          trend.startsWith("+") ? "text-green" : trend.startsWith("-") ? "text-destructive" : "text-muted-foreground"
        )}>
          {trend}
        </div>
      </div>
    </div>
  );
}

function ActivityItem({ title, desc, time, icon: Icon, status }: any) {
  return (
    <div className="p-4 flex gap-4 transition-colors hover:bg-secondary/30 group">
      <div className={cn(
        "w-8 h-8 rounded-lg grid place-items-center mt-0.5",
        status === "warning" ? "bg-amber/10 text-amber" : "bg-secondary text-muted-foreground transition-colors group-hover:text-foreground"
      )}>
        <Icon className="w-4 h-4" />
      </div>
      <div className="flex-1 space-y-1">
        <div className="flex items-center justify-between">
          <span className="text-[13px] font-bold tracking-tight">{title}</span>
          <span className="text-[11px] font-mono text-muted-foreground tabular-nums">{time}</span>
        </div>
        <p className="text-[12px] text-muted-foreground leading-relaxed">{desc}</p>
      </div>
    </div>
  );
}

function RecommendationItem({ title, desc, priority }: any) {
  return (
    <div className="space-y-1.5 group cursor-default">
      <div className="flex items-center gap-2">
        <div className={cn(
          "w-1.5 h-1.5 rounded-full",
          priority === "high" ? "bg-destructive" : priority === "med" ? "bg-amber" : "bg-blue"
        )} />
        <span className="text-[12px] font-bold tracking-tight group-hover:text-accent transition-colors">{title}</span>
      </div>
      <p className="text-[11px] text-muted-foreground pl-3.5 border-l border-border ml-0.5 group-hover:border-accent/40 transition-colors">{desc}</p>
    </div>
  );
}

function SloProgress({ name, value, percent }: any) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-[11px] font-medium">
        <span className="text-muted-foreground">{name}</span>
        <span className="font-bold tabular-nums">{value}</span>
      </div>
      <div className="h-1.5 w-full bg-secondary rounded-full overflow-hidden">
        <div 
          className={cn(
            "h-full rounded-full transition-all duration-500",
            percent > 90 ? "bg-green" : percent > 75 ? "bg-accent" : "bg-amber"
          )} 
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  );
}
