"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Shell from "@/components/layout/Shell";
import { 
  getPolicies, 
  createPolicy, 
  deletePolicy, 
  Policy 
} from "@/lib/api";
import { PolicyLogicBuilder } from "@/components/policy/PolicyLogicBuilder";
import { 
  Plus, 
  Search, 
  MoreVertical, 
  ShieldCheck, 
  Lock, 
  Unlock, 
  Trash2, 
  Info,
  Layers,
  ChevronRight,
  FileCheck
} from "lucide-react";
import { cn, formatDate } from "@/lib/utils";

const POLICY_TYPES = [
  { value: "rbac", label: "RBAC (Role-Based)", icon: ShieldCheck, color: "text-purple" },
  { value: "ip_allowlist", label: "IP Allowlist", icon: Lock, color: "text-blue" },
  { value: "data_export", label: "Data Export", icon: FileCheck, color: "text-green" },
];

export default function AccessPoliciesPage() {
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  
  // Form State
  const [name, setName] = useState("");
  const [type, setType] = useState("rbac");
  const [rules, setRules] = useState('{\n  "allowed_roles": ["admin"]\n}');

  const { data: policiesRes, status } = useQuery({
    queryKey: ["policies"],
    queryFn: () => getPolicies(),
  });

  const createMutation = useMutation({
    mutationFn: (data: any) => createPolicy(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["policies"] });
      setShowCreate(false);
      setName("");
      setType("rbac");
      setRules('{\n  "allowed_roles": ["admin"]\n}');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deletePolicy(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["policies"] });
    },
  });

  const policies = policiesRes?.data ?? [];

  return (
    <Shell 
      title="Policy Registry" 
      crumbs={["Governance", "Access Policies"]}
    >
      <div className="space-y-8 animate-fade-up">
        {/* Header Summary */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <SummaryItem label="Enforced Policies" value={policies.filter(p => p.enabled).length.toString()} />
            <SummaryItem label="Last Evaluation" value="2s ago" status="healthy" />
            <SummaryItem label="Average Latency" value="12ms" />
          </div>
          
          <button 
            className="btn btn-primary h-10 px-6 gap-2"
            onClick={() => setShowCreate(true)}
            data-testid="toggle-create-policy"
          >
            <Plus className="w-4 h-4" />
            Create global policy
          </button>
        </div>

        {/* Policies Grid */}
        <div className="grid grid-cols-1 gap-4" data-testid="policy-list">
          {status === "pending" ? (
            Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="h-24 bg-surface-1 border border-border rounded-xl animate-pulse" />
            ))
          ) : policies.length === 0 ? (
            <div className="py-20 text-center space-y-4 bg-surface-1 border border-border rounded-xl shadow-sm">
              <div className="w-16 h-16 rounded-full bg-secondary grid place-items-center mx-auto">
                <ShieldCheck className="w-8 h-8 text-muted-foreground opacity-30" />
              </div>
              <h3 className="font-bold tracking-tight">No active policies detected</h3>
              <p className="text-sm text-muted-foreground max-w-sm mx-auto leading-relaxed">
                Security policies define the global rules for access across your 
                infrastructure. Create your first RBAC or IP-based rule to begin enforcement.
              </p>
            </div>
          ) : (
            policies.map((policy) => (
              <PolicyCard 
                key={policy.id} 
                policy={policy} 
                onDelete={() => deleteMutation.mutate(policy.id)} 
              />
            ))
          )}
        </div>
      </div>

      {/* Creation Wizard / Modal */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-background/90 backdrop-blur-md animate-in fade-in duration-200">
          <div className="w-full max-w-2xl bg-surface-1 border border-border rounded-2xl shadow-2xl animate-in zoom-in-95 duration-200 overflow-hidden">
            <div className="p-8 border-b border-border bg-secondary/30">
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-xl font-bold tracking-tight">Construct Global Policy</h3>
                <button 
                  className="p-1 hover:bg-secondary rounded-lg transition-colors text-muted-foreground"
                  onClick={() => setShowCreate(false)}
                >
                  <Plus className="w-5 h-5 rotate-45" />
                </button>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                Rules are evaluated statelessly in the control plane. Modification 
                triggers a global HMAC re-signing of the policy set within 5 seconds.
              </p>
            </div>

            <div className="p-8 space-y-8">
              <div className="grid grid-cols-2 gap-8">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Policy Identity</label>
                  <input 
                    type="text" 
                    placeholder="e.g. Production Environment Access"
                    id="policy-name"
                    className={cn(
                      "w-full bg-secondary/50 border border-border rounded-xl px-4 py-3 text-[13px] font-medium outline-none transition-all",
                      "focus:ring-2 focus:ring-accent/20 focus:border-accent"
                    )}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5 text-left">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Enforcement Logic Type</label>
                  <div className="grid grid-cols-3 gap-2">
                    {POLICY_TYPES.map(t => (
                      <button 
                        key={t.value}
                        onClick={() => setType(t.value)}
                        className={cn(
                          "px-3 py-2 rounded-lg border text-[11px] font-bold transition-all transition-colors transition-colors",
                          type === t.value 
                            ? "bg-accent/10 border-accent/20 text-accent" 
                            : "bg-background border-border text-muted-foreground hover:bg-secondary"
                        )}
                      >
                        {t.label.split(" ")[0]}
                      </button>
                    ))}
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1 mb-2 block">Behavioral Configuration</label>
                <PolicyLogicBuilder 
                  type={type} 
                  value={rules} 
                  onChange={setRules} 
                />
              </div>

              <div className="flex items-center gap-4 pt-4">
                <button 
                  className="flex-1 btn btn-ghost h-12 font-bold"
                  onClick={() => setShowCreate(false)}
                >
                  Discard Draft
                </button>
                <button 
                  className="flex-[2] btn btn-primary h-12 font-bold shadow-xl shadow-accent/20"
                  onClick={() => createMutation.mutate({ name, type, rules: JSON.parse(rules), enabled: true })}
                  data-testid="submit-policy"
                >
                  {createMutation.isPending ? "Signing Policy..." : "Commit Global Rule"}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </Shell>
  );
}

function PolicyCard({ policy, onDelete }: { policy: Policy; onDelete: () => void }) {
  const typeSpec = POLICY_TYPES.find(t => t.value === policy.type) || POLICY_TYPES[0];

  return (
    <div className="group bg-surface-1 border border-border rounded-xl p-5 hover:border-accent/40 transition-all flex items-center justify-between shadow-sm hover:shadow-lg hover:shadow-accent/5">
      <div className="flex items-center gap-5">
        <div className={cn(
          "w-12 h-12 rounded-xl border border-border transition-all flex items-center justify-center",
          "group-hover:border-accent/20 group-hover:bg-accent/5"
        )}>
          <typeSpec.icon className={cn("w-5 h-5", typeSpec.color)} />
        </div>
        
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <span className="font-bold tracking-tight text-[15px]">{policy.name}</span>
            <span className={cn(
              "tag uppercase text-[9px]",
              policy.enabled ? "tag-green" : "tag-amber"
            )}>
              {policy.enabled ? "Enforcing" : "Disabled"}
            </span>
          </div>
          <div className="flex items-center gap-4 text-[12px] text-muted-foreground">
            <span className="flex items-center gap-1.5 italic font-mono decoration-accent/20 underline-offset-4 decoration-dashed underline">
              <Layers className="w-3.5 h-3.5" />
              {typeSpec.label}
            </span>
            <span className="flex items-center gap-1.5 tabular-nums">
              <Info className="w-3.5 h-3.5" />
              Active since {formatDate(policy.created_at)}
            </span>
          </div>
        </div>
      </div>

      <div className="flex items-center gap-6">
        <div className="flex flex-col items-end gap-1 px-6 border-r border-border">
          <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground opacity-60">Evaluations (24h)</span>
          <span className="text-[14px] font-bold tabular-nums">1.4k <span className="text-[11px] font-normal text-muted-foreground ml-1">Hits</span></span>
        </div>
        
        <div className="flex items-center gap-2">
          <button className="btn btn-ghost px-3 py-1.5 text-[11px] font-bold gap-1.5 border-transparent hover:border-border hover:bg-secondary">
            Settings
          </button>
          <button 
            onClick={onDelete}
            data-testid={`delete-policy-${policy.id}`}
            className="p-2 hover:bg-destructive/10 rounded-lg transition-colors text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="w-4 h-4" />
          </button>
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
        <span className="text-[16px] font-bold tabular-nums">{value}</span>
        {status === "healthy" && <div className="w-1.5 h-1.5 rounded-full bg-green animate-pulse" />}
      </div>
    </div>
  );
}
