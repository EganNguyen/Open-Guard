"use client";

import { useState } from "react";
import { 
  Code, 
  Settings, 
  ShieldCheck, 
  Info, 
  AlertCircle,
  Braces,
  Database,
  Globe,
  Users
} from "lucide-react";
import { cn } from "@/lib/utils";

interface PolicyLogicBuilderProps {
  type: string;
  value: string;
  onChange: (value: string) => void;
}

export function PolicyLogicBuilder({ type, value, onChange }: PolicyLogicBuilderProps) {
  const [activeTab, setActiveTab] = useState<"builder" | "json">("builder");

  const handleJsonChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    onChange(e.target.value);
  };

  return (
    <div className="border border-border rounded-xl bg-surface-1 overflow-hidden transition-all focus-within:ring-1 focus-within:ring-accent/40 shadow-sm animate-fade-up">
      <div className="bg-secondary/30 px-4 py-2 border-b border-border flex items-center justify-between">
        <div className="flex items-center gap-4">
          <BadgeButton 
            active={activeTab === "builder"} 
            onClick={() => setActiveTab("builder")}
            icon={Settings}
          >
            Visual Builder
          </BadgeButton>
          <BadgeButton 
            active={activeTab === "json"} 
            onClick={() => setActiveTab("json")}
            icon={Code}
          >
            Raw JSON
          </BadgeButton>
        </div>
        
        <div className="flex items-center gap-2 text-[10px] text-muted-foreground font-mono uppercase tracking-widest">
          <Database className="w-3 h-3 text-accent" />
          Enforcement: Control Plane
        </div>
      </div>

      <div className="p-0 min-h-[280px]">
        {activeTab === "json" ? (
          <textarea
            value={value}
            onChange={handleJsonChange}
            spellCheck={false}
            className="w-full h-[280px] p-6 bg-background font-mono text-[13px] text-foreground leading-relaxed focus:outline-none resize-none selection:bg-accent/20"
            placeholder='{ "rules": ... }'
          />
        ) : (
          <div className="p-8 space-y-8 bg-surface-1">
            <div className="flex items-start gap-4">
              <div className="w-8 h-8 rounded-lg bg-accent/10 grid place-items-center shrink-0">
                <ShieldCheck className="w-4 h-4 text-accent" />
              </div>
              <div className="space-y-1">
                <h4 className="text-[13px] font-bold tracking-tight">Security Logic Construction</h4>
                <p className="text-[12px] text-muted-foreground max-w-lg leading-relaxed">
                  Define the criteria for access enforcement. This policy will be evaluated 
                  statelessly across the global control plane for every inbound request.
                </p>
              </div>
            </div>

            {type === "ip_allowlist" ? (
              <VisualSection title="IP Allowlist Configuration" icon={Globe}>
                <div className="grid grid-cols-2 gap-4 pt-2">
                  <div className="space-y-1.5 p-4 bg-secondary/20 border border-border/50 rounded-lg">
                    <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">Network Range (CIDR)</span>
                    <input 
                      type="text" 
                      placeholder="e.g. 192.168.1.0/24" 
                      className="bg-background border border-border px-3 py-1.5 rounded text-[13px] w-full font-mono"
                    />
                  </div>
                  <div className="p-4 bg-secondary/10 border border-border/30 rounded-lg flex flex-col justify-center gap-1.5">
                    <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">Active Peers</span>
                    <span className="text-[18px] font-bold tabular-nums">0/256 <span className="text-[11px] font-medium text-muted-foreground ml-1">Allowed</span></span>
                  </div>
                </div>
              </VisualSection>
            ) : type === "rbac" ? (
              <VisualSection title="Role-Based Enforcement" icon={Users}>
                <div className="space-y-4 pt-2">
                  <div className="p-4 bg-secondary/20 border border-border/50 rounded-lg space-y-3">
                    <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground block">Required Roles</span>
                    <div className="flex gap-2 flex-wrap">
                      {["admin", "security_ops", "compliance_bot"].map(role => (
                        <span key={role} className="tag tag-blue cursor-default">
                          {role}
                        </span>
                      ))}
                      <button className="tag bg-accent/20 border border-accent/20 text-accent font-bold hover:bg-accent/30 transition-colors">+ Add Role</button>
                    </div>
                  </div>
                </div>
              </VisualSection>
            ) : (
              <div className="flex flex-col items-center justify-center p-12 space-y-4 text-center">
                <Braces className="w-10 h-10 text-muted-foreground opacity-20" />
                <div className="space-y-1">
                  <p className="text-[13px] font-medium text-muted-foreground">Visual builder unavailable for this policy type</p>
                  <p className="text-[11px] text-muted-foreground/60 max-w-[240px]">Please use the Raw JSON editor to manually configure these rules.</p>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      <div className="px-6 py-4 border-t border-border bg-secondary/20 flex items-center gap-3">
        <Info className="w-4 h-4 text-accent shrink-0" />
        <p className="text-[11px] text-muted-foreground leading-relaxed">
          OpenGuard uses <strong>E-RBAC</strong> (External RBAC) with a stateless JSONB 
          schema. Modifications to this policy are applied globally within 5 seconds.
        </p>
      </div>
    </div>
  );
}

function BadgeButton({ active, onClick, icon: Icon, children }: any) {
  return (
    <button 
      onClick={onClick}
      className={cn(
        "flex items-center gap-2 px-3 py-1 rounded-md text-[11px] font-bold transition-all border border-transparent whitespace-nowrap",
        active 
          ? "bg-accent/10 border-accent/20 text-accent" 
          : "text-muted-foreground hover:bg-secondary hover:text-foreground"
      )}
    >
      <Icon className={cn("w-3.5 h-3.5", active ? "text-accent" : "text-muted-foreground")} />
      {children}
    </button>
  );
}

function VisualSection({ title, icon: Icon, children }: any) {
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Icon className="w-3.5 h-3.5 text-muted-foreground" />
        <span className="text-[12px] font-bold tracking-tight text-foreground">{title}</span>
      </div>
      {children}
    </div>
  );
}
