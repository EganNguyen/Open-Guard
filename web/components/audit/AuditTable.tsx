"use client";

import * as React from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { AuditEvent } from "@/lib/api";
import { cn, formatDate } from "@/lib/utils";
import { ChevronDown, ChevronRight, Shield, User, Globe, Terminal } from "lucide-react";

interface AuditTableProps {
  events: AuditEvent[];
  onLoadMore?: () => void;
  isFetchingNextPage?: boolean;
}

export function AuditTable({ events, onLoadMore, isFetchingNextPage }: AuditTableProps) {
  const parentRef = React.useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 52,
    overscan: 10,
  });

  const [expandedId, setExpandedId] = React.useState<string | null>(null);

  return (
    <div 
      ref={parentRef}
      className="h-[600px] overflow-auto border border-border rounded-lg bg-surface-1"
    >
      <div
        className="w-full relative"
        style={{ height: `${rowVirtualizer.getTotalSize()}px` }}
      >
        <table className="w-full text-left border-collapse">
          <thead className="sticky top-0 bg-secondary/80 backdrop-blur z-10 border-b border-border">
            <tr className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-3 w-10"></th>
              <th className="px-4 py-3 w-48">Timestamp</th>
              <th className="px-4 py-3">Actor</th>
              <th className="px-4 py-3 w-40">Event Type</th>
              <th className="px-4 py-3 w-24 text-right">Status</th>
            </tr>
          </thead>
          <tbody>
            {rowVirtualizer.getVirtualItems().map((virtualRow) => {
              const event = events[virtualRow.index];
              const isExpanded = expandedId === event.id;
              
              return (
                <React.Fragment key={event.id}>
                  <tr 
                    data-index={virtualRow.index}
                    onClick={() => setExpandedId(isExpanded ? null : event.id)}
                    className={cn(
                      "group cursor-pointer hover:bg-secondary/50 transition-colors border-b border-border/50 last:border-0 h-[52px]",
                      isExpanded && "bg-secondary/30"
                    )}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                  >
                    <td className="px-4 text-muted-foreground group-hover:text-foreground transition-colors">
                      {isExpanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                    </td>
                    <td className="px-4 text-[12px] font-mono text-muted-foreground tabular-nums">
                      {formatDate(event.occurred_at)}
                    </td>
                    <td className="px-4">
                      <div className="flex items-center gap-2">
                        <div className="w-6 h-6 rounded-full bg-secondary grid place-items-center">
                          {event.actor_type === "user" ? <User className="w-3 h-3" /> : <Shield className="w-3 h-3" />}
                        </div>
                        <span className="text-[13px] font-medium max-w-[200px] truncate">
                          {event.actor_email || event.actor_id}
                        </span>
                      </div>
                    </td>
                    <td className="px-4">
                      <span className="text-[12px] font-mono bg-secondary px-2 py-0.5 rounded border border-border/50">
                        {event.type}
                      </span>
                    </td>
                    <td className="px-4 text-right">
                      <span className={cn(
                        "tag",
                        event.status === "success" ? "tag-green" : "tag-red"
                      )}>
                        {event.status}
                      </span>
                    </td>
                  </tr>
                  {isExpanded && (
                    <tr 
                      className="bg-secondary/20 border-b border-border"
                      style={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        width: '100%',
                        transform: `translateY(${virtualRow.start + 52}px)`,
                      }}
                    >
                      <td colSpan={5} className="p-4">
                        <div className="bg-background rounded-md border border-border p-4 space-y-4">
                          <div className="flex items-center gap-6 text-[12px]">
                            <div className="flex items-center gap-2 text-muted-foreground">
                              <Terminal className="w-3.5 h-3.5" />
                              <span className="font-bold">Request ID:</span>
                              <span className="font-mono text-foreground">{event.request_id || "N/A"}</span>
                            </div>
                            <div className="flex items-center gap-2 text-muted-foreground">
                              <Globe className="w-3.5 h-3.5" />
                              <span className="font-bold">Source IP:</span>
                              <span className="font-mono text-foreground">{(event.metadata as any)?.ip || "Unknown"}</span>
                            </div>
                          </div>
                          
                          <div className="space-y-1.5">
                            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Payload</span>
                            <pre className="text-[11px] font-mono bg-secondary/50 p-3 rounded border border-border/50 overflow-x-auto">
                              {JSON.stringify(event.metadata || {}, null, 2)}
                            </pre>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
      
      {onLoadMore && (
        <div className="p-4 flex justify-center border-t border-border bg-secondary/20">
          <button 
            onClick={onLoadMore}
            disabled={isFetchingNextPage}
            className="btn btn-ghost text-[12px]"
          >
            {isFetchingNextPage ? "Loading more..." : "Load more events"}
          </button>
        </div>
      )}
    </div>
  );
}
