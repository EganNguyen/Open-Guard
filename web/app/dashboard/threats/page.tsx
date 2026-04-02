"use client";

import Shell from "@/components/layout/Shell";
import { 
  Activity, 
  ShieldAlert, 
  Zap, 
  MapPin, 
  User, 
  Terminal,
  MoreVertical,
  History,
  AlertTriangle,
  Play
} from "lucide-react";
import { cn } from "@/lib/utils";

export default function ThreatsPage() {
  return (
    <Shell 
      title="Threat Intelligence" 
      crumbs={["Security", "Threat Detection"]}
    >
      <div className="space-y-6 animate-fade-up">
        {/* Header Summary */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <SummaryItem label="Active Detections" value="3" status="critical" />
            <SummaryItem label="Global Threat Level" value="Level 2 (Elevated)" />
            <SummaryItem label="Mean Time to Resolve" value="42m" />
          </div>
          
          <div className="flex gap-3">
            <button className="btn btn-ghost text-[12px] gap-2">
              <History className="w-3.5 h-3.5" />
              History
            </button>
            <button className="btn btn-primary text-[12px] gap-2 bg-destructive hover:bg-red-600 border-none">
              <Zap className="w-3.5 h-3.5" />
              Simulate Attack
            </button>
          </div>
        </div>

        {/* Live Stream Section */}
        <div className="border border-border rounded-xl bg-surface-1 overflow-hidden shadow-sm">
          <div className="bg-secondary/30 px-6 py-4 border-b border-border flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 rounded-full bg-red animate-pulse" />
              <h3 className="text-sm font-bold tracking-tight uppercase tracking-widest text-muted-foreground mr-4">Live Threat Feed</h3>
              <span className="text-[10px] bg-red/10 text-red px-2 py-0.5 rounded-full font-bold">SSE Connected</span>
            </div>
            <div className="flex items-center gap-4 text-[11px] text-muted-foreground">
              <span className="flex items-center gap-1.5 font-bold uppercase tracking-wider tabular-nums tracking-widest">3 events in last 5m</span>
            </div>
          </div>

          <div className="divide-y divide-border">
            <ThreatItem 
              severity="critical"
              title="Brute Force Authentication"
              desc="Multiple failed login attempts detected against managed account: bob@acme.com"
              location="San Jose, CA (Datacenter)"
              time="2m ago"
              actor="192.168.4.120"
              traceId="t_882jxl09"
            />
            <ThreatItem 
              severity="high"
              title="Impossible Travel"
              desc="Active session detected from Tokyo, JP shortly after San Francisco, US access."
              location="Tokyo, JP (Mobile Network)"
              time="18m ago"
              actor="jane@acme.com"
              traceId="t_991kz44s"
            />
            <ThreatItem 
              severity="medium"
              title="Privilege Escalation Attempt"
              desc="Unauthorized call to admin.connectors.delete from service-worker identity."
              location="us-east-1 (Internal Relay)"
              time="1h ago"
              actor="svc-infra-3"
              traceId="t_112mx91a"
            />
          </div>
        </div>

        {/* Threat Map Placeholder */}
        <div className="grid grid-cols-3 gap-6">
          <div className="col-span-2 h-[300px] bg-secondary/10 border border-border rounded-xl flex flex-col items-center justify-center space-y-4">
             <div className="w-12 h-12 rounded-full border-2 border-accent/20 border-t-accent animate-spin" />
             <div className="text-center space-y-1">
               <p className="text-[13px] font-bold tracking-tight">Loading Global Threat Map</p>
               <p className="text-[11px] text-muted-foreground">Processing 1.2M events from ClickHouse materialize view...</p>
             </div>
          </div>
          <div className="space-y-4">
              <aside className="p-5 bg-card border border-border rounded-xl space-y-4 shadow-sm">
                <h4 className="text-sm font-bold tracking-tight flex items-center gap-2">
                  <Play className="w-3.5 h-3.5 text-accent" />
                  Saga Enforcement
                </h4>
                <p className="text-[11px] text-muted-foreground leading-relaxed leading-relaxed">
                  OpenGuard automated sagas are currently protecting your perimeter. 
                  Suspicious traffic is automatically redirected through the Honeypot 
                  Control Plane for deep packet analysis.
                </p>
              </aside>
              <aside className="p-5 bg-destructive/5 border border-destructive/20 rounded-xl space-y-4 shadow-sm">
                <h4 className="text-sm font-bold tracking-tight text-destructive flex items-center gap-2">
                  <AlertTriangle className="w-3.5 h-3.5" />
                  Incident Response
                </h4>
                <button className="w-full btn btn-primary bg-destructive hover:bg-red-600 border-none py-2 text-[11px] font-bold">
                  Open War Room
                </button>
              </aside>
          </div>
        </div>
      </div>
    </Shell>
  );
}

function ThreatItem({ severity, title, desc, location, time, actor, traceId }: any) {
  return (
    <div className="group p-6 hover:bg-secondary/20 transition-all cursor-default">
      <div className="flex items-start justify-between">
        <div className="flex gap-5">
           <div className={cn(
             "w-10 h-10 rounded-xl border flex items-center justify-center shrink-0 transition-all",
             severity === "critical" ? "bg-red/10 border-red/20 text-red" : 
             severity === "high" ? "bg-amber/10 border-amber/20 text-amber" :
             "bg-secondary border-border text-muted-foreground group-hover:bg-accent/10 group-hover:border-accent/20 group-hover:text-accent"
           )}>
             <ShieldAlert className="w-5 h-5" />
           </div>
           
           <div className="space-y-1.5 flex-1 max-w-xl">
              <div className="flex items-center gap-3">
                <h4 className="text-[14px] font-bold tracking-tight">{title}</h4>
                <span className={cn(
                  "tag uppercase tracking-widest text-[9px]",
                  severity === "critical" ? "tag-red" : severity === "high" ? "tag-amber" : "tag-purple"
                )}>
                  {severity}
                </span>
              </div>
              <p className="text-[12px] text-muted-foreground leading-relaxed italic">{desc}</p>
              
              <div className="flex items-center gap-6 pt-1 text-[11px] text-muted-foreground font-medium tabular-nums shadow-sm shadow-accent/5">
                <span className="flex items-center gap-1.5">
                  <MapPin className="w-3 h-3 text-accent" />
                  {location}
                </span>
                <span className="flex items-center gap-1.5">
                  <User className="w-3 h-3 text-accent" />
                  {actor}
                </span>
              </div>
           </div>
        </div>

        <div className="flex flex-col items-end gap-3 self-center">
            <span className="text-[11px] font-mono text-muted-foreground tabular-nums tracking-widest">{time}</span>
            <div className="flex items-center gap-2">
              <div className="flex items-center gap-1 bg-surface-1 border border-border px-2 py-0.5 rounded text-[10px] font-mono shadow-sm group-hover:border-accent/40 transition-colors">
                <Terminal className="w-2.5 h-2.5 text-muted-foreground" />
                {traceId}
              </div>
              <button className="p-1.5 hover:bg-secondary rounded p-1.5 opacity-0 group-hover:opacity-100 transition-all">
                <MoreVertical className="w-4 h-4 text-muted-foreground" />
              </button>
            </div>
        </div>
      </div>
    </div>
  );
}

function SummaryItem({ label, value, status }: any) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">{label}</span>
      <div className="flex items-center gap-2">
        <span className={cn(
          "text-[18px] font-bold tabular-nums tracking-tight",
          status === "critical" && "text-destructive"
        )}>{value}</span>
        {status === "critical" && <div className="w-1.5 h-1.5 rounded-full bg-red animate-pulse" />}
      </div>
    </div>
  );
}
