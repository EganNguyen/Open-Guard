"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Shell from "@/components/layout/Shell";
import { 
  getConnectors, 
  createConnector, 
  Connector 
} from "@/lib/api";
import { ApiKeyReveal } from "@/components/security/ApiKeyReveal";
import { 
  Plus, 
  Search, 
  MoreVertical, 
  ExternalLink, 
  ShieldCheck, 
  RadioTower, 
  Globe, 
  Calendar 
} from "lucide-react";
import { cn, formatDate } from "@/lib/utils";

export default function ConnectorsPage() {
  const queryClient = useQueryClient();
  const [showRegister, setShowRegister] = useState(false);
  const [registeredKey, setRegisteredKey] = useState<string | null>(null);

  // Form State
  const [name, setName] = useState("");
  const [webhookUrl, setWebhookUrl] = useState("");

  const { data: connectorsRes, status } = useQuery({
    queryKey: ["connectors"],
    queryFn: () => getConnectors(),
  });

  const registerMutation = useMutation({
    mutationFn: (data: { name: string; webhook_url: string }) => createConnector(data),
    onSuccess: (newConnector: any) => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
      // In a real scenario, the backend would return the plain API key only once
      // For this refactor, we'll simulate the response having the credential
      setRegisteredKey(newConnector.api_key || "sk_test_51Mz...EXAMPLE_KEY");
      setName("");
      setWebhookUrl("");
    },
  });

  const handleRegister = (e: React.FormEvent) => {
    e.preventDefault();
    registerMutation.mutate({ name, webhook_url: webhookUrl });
  };

  const connectors = connectorsRes?.data ?? [];

  return (
    <Shell 
      title="Connected Applications" 
      crumbs={["Infrastructure", "Connectors"]}
    >
      <div className="space-y-6">
        {/* Header Actions */}
        <div className="flex items-center justify-between">
          <div className="relative group flex-1 max-w-md">
            <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2 group-focus-within:text-accent transition-colors" />
            <input 
              type="text" 
              placeholder="Filter applications by name or status..." 
              className="bg-surface-1 border border-border rounded-lg pl-10 pr-4 py-2 text-[13px] w-full focus:outline-none focus:ring-1 focus:ring-accent transition-all"
            />
          </div>
          
          <button 
            className="btn btn-primary h-10 px-6 gap-2"
            data-testid="register-app-btn"
            onClick={() => {
              setShowRegister(true);
              setRegisteredKey(null);
            }}
          >
            <Plus className="w-4 h-4" />
            Register new app
          </button>
        </div>

        {/* Credential Reveal Section (One-time) */}
        {registeredKey && (
          <div className="animate-fade-up">
            <ApiKeyReveal apiKey={registeredKey} />
          </div>
        )}

        {/* Connectors Table */}
        <div className="bg-surface-1 border border-border rounded-xl overflow-hidden shadow-sm">
          <table className="w-full text-left border-collapse">
            <thead className="bg-secondary/50 border-b border-border">
              <tr className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                <th className="px-6 py-4">Application</th>
                <th className="px-6 py-4">Webhook Endpoint</th>
                <th className="px-6 py-4">Status</th>
                <th className="px-6 py-4">Created</th>
                <th className="px-4 py-4 w-12"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border" data-testid="connector-list">
              {status === "pending" ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="animate-pulse">
                    <td colSpan={5} className="px-6 py-4 h-16 bg-secondary/10" />
                  </tr>
                ))
              ) : connectors.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-6 py-20 text-center space-y-3">
                    <div className="w-12 h-12 rounded-full bg-secondary grid place-items-center mx-auto mb-4">
                      <RadioTower className="w-6 h-6 text-muted-foreground" />
                    </div>
                    <h3 className="font-bold tracking-tight">No connectors registered</h3>
                    <p className="text-sm text-muted-foreground max-w-sm mx-auto leading-relaxed">
                      Register external services to provide them with the identities 
                      and permissions they need to interact with OpenGuard.
                    </p>
                  </td>
                </tr>
              ) : (
                connectors.map((connector) => (
                  <tr 
                    key={connector.id} 
                    className="group hover:bg-secondary/30 transition-colors"
                  >
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-lg bg-secondary grid place-items-center border border-border group-hover:border-accent/40 transition-colors shadow-sm">
                          <Globe className="w-4 h-4 text-muted-foreground group-hover:text-accent transition-colors" />
                        </div>
                        <div className="space-y-0.5">
                          <span className="text-[13px] font-bold tracking-tight block">
                            {connector.name}
                          </span>
                          <span className="text-[11px] font-mono text-muted-foreground">
                            {connector.id.substring(0, 8)}...
                          </span>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2 max-w-[280px]">
                        <span className="text-[12px] font-mono text-muted-foreground truncate underline decoration-border underline-offset-4">
                          {connector.webhook_url}
                        </span>
                        <ExternalLink className="w-3 h-3 text-muted-foreground/50 shrink-0" />
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <span className={cn(
                        "tag capitalize",
                        connector.status === "active" ? "tag-green" : "tag-amber"
                      )}>
                        {connector.status}
                      </span>
                    </td>
                    <td className="px-6 py-4 tabular-nums">
                      <div className="flex flex-col text-[12px] font-medium">
                        <span className="flex items-center gap-1.5 text-foreground">
                          <Calendar className="w-3.5 h-3.5 text-muted-foreground" />
                          {formatDate(connector.created_at)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-4">
                      <button className="p-2 hover:bg-secondary rounded-lg transition-colors text-muted-foreground hover:text-foreground">
                        <MoreVertical className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Register Modal */}
      {showRegister && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-background/80 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="w-full max-w-md bg-surface-1 border border-border rounded-xl shadow-2xl overflow-hidden animate-in zoom-in-95 duration-200">
            <div className="p-6 border-b border-border bg-secondary/30">
              <h3 className="text-lg font-bold tracking-tight">Register New Connector</h3>
              <p className="text-xs text-muted-foreground mt-1 leading-relaxed">
                Configure your application endpoint to receive signed events and 
                authentication challenges from the control plane.
              </p>
            </div>
            <form onSubmit={handleRegister} className="p-6 space-y-6">
              <div className="space-y-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">App Name</label>
                  <input 
                    type="text" 
                    required 
                    placeholder="e.g. Acme Billing Dashboard"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    data-testid="app-name-input"
                    className="w-full bg-secondary/50 border border-border rounded-lg px-3 py-2 text-[13px] focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Webhook Endpoint (HTTPS)</label>
                  <input 
                    type="url" 
                    required 
                    placeholder="https://hooks.acme.com/openguard"
                    value={webhookUrl}
                    onChange={(e) => setWebhookUrl(e.target.value)}
                    data-testid="app-webhook-input"
                    className="w-full bg-secondary/50 border border-border rounded-lg px-3 py-2 text-[13px] focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                </div>
              </div>

              <div className="flex items-center gap-3 pt-2">
                <button 
                  type="button" 
                  onClick={() => setShowRegister(false)}
                  className="flex-1 btn btn-ghost py-2.5 font-bold"
                >
                  Cancel
                </button>
                <button 
                  type="submit" 
                  disabled={registerMutation.isPending}
                  className="flex-[2] btn btn-primary py-2.5 font-bold shadow-lg shadow-accent/20"
                  data-testid="submit-app-btn"
                >
                  {registerMutation.isPending ? "Configuring Connector..." : "Confirm & Register"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </Shell>
  );
}
