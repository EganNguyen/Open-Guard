"use client";

import Shell from "@/components/layout/Shell";
import { AuditTable } from "@/components/audit/AuditTable";
import { useAuditEvents } from "@/hooks/use-audit-events";
import { Download, RefreshCw, ShieldAlert, History } from "lucide-react";

export default function AuditPage() {
  const { 
    data, 
    fetchNextPage, 
    hasNextPage, 
    isFetchingNextPage, 
    status,
    refetch 
  } = useAuditEvents();

  const events = data?.pages.flatMap((page) => page.data) ?? [];

  return (
    <Shell 
      title="Audit Log" 
      crumbs={["Governance", "Audit Log"]}
    >
      <div className="flex flex-col gap-6">
        {/* Header Actions */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1.5 bg-secondary/50 px-3 py-1.5 rounded-md border border-border text-[12px] font-medium">
              <History className="w-3.5 h-3.5 text-muted-foreground" />
              <span>Streaming events</span>
              <div className="w-1.5 h-1.5 rounded-full bg-green animate-pulse ml-1" />
            </div>
          </div>
          
          <div className="flex items-center gap-3">
            <button 
              className="btn btn-ghost text-[12px] gap-2"
              onClick={() => refetch()}
            >
              <RefreshCw className="w-3.5 h-3.5" />
              Refresh
            </button>
            <button className="btn btn-ghost text-[12px] gap-2">
              <Download className="w-3.5 h-3.5" />
              Export
            </button>
            <button className="btn btn-primary text-[12px] gap-2">
              <ShieldAlert className="w-3.5 h-3.5" />
              Integrity Check
            </button>
          </div>
        </div>

        {/* Stats Summary */}
        <div className="grid grid-cols-4 gap-4">
          <div className="p-4 bg-surface-1 border border-border rounded-lg space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Total Events (24h)</span>
            <div className="text-2xl font-bold">12,482</div>
            <div className="text-[11px] text-green font-medium">+14% vs yesterday</div>
          </div>
          <div className="p-4 bg-surface-1 border border-border rounded-lg space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Critical Failures</span>
            <div className="text-2xl font-bold text-destructive">24</div>
            <div className="text-[11px] text-destructive font-medium">3 requiring review</div>
          </div>
          <div className="p-4 bg-surface-1 border border-border rounded-lg space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Integrity Status</span>
            <div className="text-2xl font-bold text-green">Verified</div>
            <div className="text-[11px] text-muted-foreground">Last checked 5m ago</div>
          </div>
          <div className="p-4 bg-surface-1 border border-border rounded-lg space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Storage Usage</span>
            <div className="text-2xl font-bold text-accent">1.2 GB</div>
            <div className="text-[11px] text-muted-foreground">90 days retention</div>
          </div>
        </div>

        {/* Audit Table */}
        {status === "pending" ? (
          <div className="h-[400px] flex flex-col items-center justify-center gap-4 border border-border rounded-lg bg-surface-1">
            <div className="w-10 h-10 border-4 border-accent border-t-transparent rounded-full animate-spin" />
            <span className="text-muted-foreground animate-pulse text-[13px]">Stream processing audit trail...</span>
          </div>
        ) : status === "error" ? (
          <div className="h-[200px] flex flex-col items-center justify-center gap-4 border border-destructive/20 rounded-lg bg-destructive/5">
            <span className="text-destructive font-medium">Failed to load audit history</span>
            <button className="btn btn-primary" onClick={() => refetch()}>Try again</button>
          </div>
        ) : (
          <AuditTable 
            events={events} 
            onLoadMore={hasNextPage ? () => fetchNextPage() : undefined}
            isFetchingNextPage={isFetchingNextPage}
          />
        )}
      </div>
    </Shell>
  );
}
