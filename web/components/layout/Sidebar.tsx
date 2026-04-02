"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { 
  LayoutDashboard, 
  Activity, 
  ShieldCheck, 
  RadioTower, 
  Users, 
  FileCheck, 
  Settings,
  Database,
  History
} from "lucide-react";
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const NAV_ITEMS = [
  { name: "Overview", href: "/dashboard", icon: LayoutDashboard },
  { name: "Audit log", href: "/dashboard/audit", icon: History },
  { name: "Threats", href: "/dashboard/threats", icon: Activity },
  { name: "Access policies", href: "/dashboard/access-policies", icon: ShieldCheck },
  { name: "Connectors", href: "/dashboard/connectors", icon: RadioTower },
  { name: "Managed users", href: "/dashboard/users", icon: Users },
  { name: "Compliance", href: "/dashboard/compliance", icon: FileCheck },
  { name: "Organization", href: "/dashboard/settings", icon: Settings },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="w-64 border-r border-border bg-sidebar h-screen flex flex-col sticky top-0 overflow-y-auto">
      <div className="p-6 flex items-center gap-2">
        <div className="w-8 h-8 rounded bg-accent grid place-items-center">
          <ShieldCheck className="w-5 h-5 text-accent-foreground" />
        </div>
        <span className="font-bold text-lg tracking-tight">OpenGuard</span>
      </div>

      <nav className="flex-1 px-3 space-y-1">
        {NAV_ITEMS.map((item) => {
          const isActive = pathname === item.href;
          return (
            <Link
              key={item.name}
              href={item.href}
              className={cn(
                "flex items-center gap-3 px-3 py-2 rounded-md transition-colors text-[13px] font-medium",
                isActive 
                  ? "bg-accent/10 text-accent" 
                  : "text-muted-foreground hover:bg-secondary hover:text-foreground"
              )}
            >
              <item.icon className={cn("w-4 h-4", isActive ? "text-accent" : "text-muted-foreground")} />
              {item.name}
            </Link>
          );
        })}
      </nav>

      <div className="p-4 border-t border-border mt-auto">
        <div className="bg-secondary/50 rounded-lg p-3">
          <div className="flex items-center gap-2 mb-2">
            <Database className="w-3 h-3 text-muted-foreground" />
            <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">System Health</span>
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between text-[11px]">
              <span className="text-muted-foreground">Kafka Consumer</span>
              <span className="text-green flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-green" /> Healthy
              </span>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}
