"use client";

import Shell from "@/components/layout/Shell";
import { 
  FileCheck, 
  ShieldCheck, 
  Download, 
  RefreshCw, 
  Info,
  Calendar,
  Layers,
  FileText,
  BadgeCheck,
  AlertCircle
} from "lucide-react";
import { cn } from "@/lib/utils";

export default function CompliancePage() {
  return (
    <Shell 
      title="Compliance & Governance" 
      crumbs={["Risk Management", "Compliance"]}
    >
      <div className="space-y-10 animate-fade-up">
        {/* Header Summary */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-10">
            <SummaryItem label="Overall Compliance" value="92%" status="healthy" />
            <SummaryItem label="Total Controls" value="124" />
            <SummaryItem label="Audit Window" value="90 Days" />
          </div>
          
          <button className="btn btn-primary h-11 px-8 font-bold gap-3 shadow-lg shadow-accent/20">
            <RefreshCw className="w-4 h-4" />
            Execute Full Audit
          </button>
        </div>

        {/* Standards Grid */}
        <div className="grid grid-cols-3 gap-6">
          <StandardCard 
            title="SOC 2 Type II" 
            desc="Security, Availability, Processing Integrity, Confidentiality, and Privacy."
            progress={94}
            status="compliant"
            lastAudit="Mar 12, 2024"
          />
          <StandardCard 
            title="GDPR Compliance" 
            desc="EU General Data Protection Regulation requirements for user data privacy."
            progress={100}
            status="compliant"
            lastAudit="Feb 28, 2024"
          />
          <StandardCard 
            title="HIPAA / HITECH" 
            desc="Rules for protecting Sensitive Patient Health Information (PHI)."
            progress={82}
            status="at-risk"
            lastAudit="Jan 15, 2024"
            warning="3 controls missing documentation"
          />
        </div>

        {/* Detailed Controls Panel */}
        <div className="bg-surface-1 border border-border rounded-2xl overflow-hidden shadow-sm">
          <div className="bg-secondary/30 px-8 py-5 border-b border-border flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Layers className="w-4 h-4 text-accent" />
              <h3 className="font-bold tracking-tight">Active Governance Controls</h3>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground mr-4">Filter:</span>
              <button className="tag tag-blue active">Administrative</button>
              <button className="tag bg-secondary border-border text-muted-foreground">Technical</button>
              <button className="tag bg-secondary border-border text-muted-foreground">Physical</button>
            </div>
          </div>

          <div className="divide-y divide-border">
            <ControlItem 
              id="AC-1" 
              title="Identity & Access Management" 
              desc="Role-based access controls (RBAC) must be enforced across all production endpoints."
              status="pass"
              coverage="98%"
            />
            <ControlItem 
              id="AU-4" 
              title="Audit Storage & Retention" 
              desc="Cryptographic evidence of all management actions must be retained for at least 90 days."
              status="pass"
              coverage="100%"
            />
            <ControlItem 
              id="IA-5" 
              title="MFA Enforcement" 
              desc="Multi-factor authentication must be enabled for all administrative and user accounts."
              status="warn"
              coverage="84%"
            />
            <ControlItem 
              id="SC-7" 
              title="Boundary Protection" 
              desc="All organization-owned traffic must be inspected for PII/SSN patterns before egress."
              status="pass"
              coverage="100%"
            />
          </div>
        </div>

        {/* Export / Report Section */}
        <section className="bg-card border border-border rounded-2xl p-8 flex items-center justify-between group cursor-default shadow-sm hover:border-accent/40 transition-all">
          <div className="flex items-center gap-6">
            <div className="w-14 h-14 rounded-2xl bg-secondary grid place-items-center group-hover:scale-105 transition-transform transition-colors shadow-sm group-hover:border-accent/20 border border-transparent">
              <FileCheck className="w-7 h-7 text-muted-foreground group-hover:text-accent transition-colors" />
            </div>
            <div className="space-y-1.5 text-left">
              <h3 className="font-bold tracking-tight text-lg leading-tight">Quarterly Posture Summary (Q1 2024)</h3>
              <p className="text-[12px] text-muted-foreground max-w-sm leading-relaxed">
                Comprehensive PDF report detailing audit logs, threat detections, and 
                policy evaluation results signed by the OpenGuard Control Plane.
              </p>
            </div>
          </div>
          <button className="btn btn-ghost h-12 px-8 font-bold gap-3 border-accent/20 group-hover:bg-accent/5 group-hover:border-accent transition-all">
            <Download className="w-4 h-4" />
            Download Signed PDF
          </button>
        </section>
      </div>
    </Shell>
  );
}

function StandardCard({ title, desc, progress, status, lastAudit, warning }: any) {
  return (
    <div className="p-6 bg-surface-1 border border-border rounded-2xl space-y-6 hover:border-accent/40 transition-all group shadow-sm transition-all hover:shadow-lg hover:shadow-accent/5">
      <div className="flex items-center justify-between">
        <h4 className="font-bold tracking-tight text-[15px]">{title}</h4>
        <div className={cn(
          "tag uppercase tracking-widest text-[9px] font-bold px-2 py-0.5 whitespace-nowrap",
          status === "compliant" ? "tag-green" : "tag-amber"
        )}>
          {status}
        </div>
      </div>
      <p className="text-[12px] text-muted-foreground leading-relaxed italic line-clamp-2">{desc}</p>
      
      <div className="space-y-2">
        <div className="flex items-center justify-between text-[11px] font-bold">
          <span className="text-muted-foreground uppercase tracking-wider">Implementation Progress</span>
          <span className={cn(status === "compliant" ? "text-green" : "text-amber")}>{progress}%</span>
        </div>
        <div className="h-2 w-full bg-secondary rounded-full overflow-hidden border border-border/50 shadow-inner">
          <div 
            className={cn(
              "h-full rounded-full transition-all duration-700",
              status === "compliant" ? "bg-green shadow-[0_0_8px_rgba(34,217,143,0.3)]" : "bg-amber"
            )} 
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      <div className="flex items-center justify-between pt-4 border-t border-border mt-6">
        <div className="flex items-center gap-1.5 text-[10px] font-mono text-muted-foreground tabular-nums">
          <Calendar className="w-3 h-3" />
          Last Audit: {lastAudit}
        </div>
        {warning && (
          <div className="flex items-center gap-1 text-[10px] font-bold text-amber">
            <AlertCircle className="w-3 h-3 group-hover:animate-bounce" />
            Attention
          </div>
        )}
      </div>
    </div>
  );
}

function ControlItem({ id, title, desc, status, coverage }: any) {
  return (
    <div className="px-8 py-6 group hover:bg-secondary/20 transition-all cursor-default">
      <div className="flex items-start justify-between">
        <div className="flex gap-6">
           <div className="w-8 h-8 rounded-lg bg-secondary flex items-center justify-center shrink-0 border border-border group-hover:border-accent/40 group-hover:bg-accent/5 transition-all transition-colors transition-colors">
              <span className="text-[10px] font-mono font-bold text-muted-foreground group-hover:text-accent tabular-nums">{id}</span>
           </div>
           <div className="space-y-1.5 flex-1 max-w-xl">
              <div className="flex items-center gap-3">
                <h4 className="text-[13px] font-bold tracking-tight">{title}</h4>
                {status === "pass" ? (
                  <BadgeCheck className="w-4 h-4 text-green" />
                ) : (
                  <AlertCircle className="w-4 h-4 text-amber" />
                )}
              </div>
              <p className="text-[12px] text-muted-foreground leading-relaxed">{desc}</p>
           </div>
        </div>
        <div className="flex items-center gap-10">
          <div className="flex flex-col items-end gap-1 px-8 border-r border-border">
            <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground opacity-60">Coverage</span>
            <span className={cn("text-[14px] font-bold tabular-nums", status === "pass" ? "text-green" : "text-amber")}>{coverage}</span>
          </div>
          <button className="p-2 hover:bg-secondary rounded-lg transition-colors text-muted-foreground hover:text-foreground">
             <FileText className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
}

function SummaryItem({ label, value, status }: any) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-widest">{label}</span>
      <div className="flex items-center gap-2">
        <span className="text-[20px] font-bold tabular-nums tracking-tighter transition-colors">{value}</span>
        {status === "healthy" && <div className="w-1.5 h-1.5 rounded-full bg-green animate-pulse" />}
      </div>
    </div>
  );
}
