"use client";

import Sidebar from "./Sidebar";
import { Search, Bell, HelpCircle, ChevronRight, User } from "lucide-react";
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

interface ShellProps {
  children: React.ReactNode;
  title?: string;
  crumbs?: string[];
}

export default function Shell({ children, title, crumbs }: ShellProps) {
  return (
    <div className="flex bg-background min-h-screen">
      <Sidebar />
      <div className="flex-1 flex flex-col min-w-0 h-screen overflow-hidden">
        {/* Topbar */}
        <header className="h-16 border-b border-border bg-card/50 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-20">
          <div className="flex items-center gap-4 text-[13px]">
            <div className="flex items-center gap-2 text-muted-foreground">
              <span data-testid="page-title">OpenGuard</span>
              {crumbs?.map((crumb, i) => (
                <div key={i} className="flex items-center gap-2">
                  <ChevronRight className="w-3.5 h-3.5" />
                  <span 
                    data-testid={`crumb-${i}`}
                    className={cn(i === crumbs.length - 1 && "text-foreground font-medium")}
                  >
                    {crumb}
                  </span>
                </div>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-4">
            <div className="relative group">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2 group-focus-within:text-accent transition-colors" />
              <input 
                type="text" 
                placeholder="Search resources..." 
                className="bg-secondary/50 border border-border rounded-md pl-9 pr-3 py-1.5 text-[12px] w-64 focus:outline-none focus:ring-1 focus:ring-accent focus:border-accent transition-all"
              />
            </div>
            
            <div className="flex items-center gap-2 border-l border-border pl-4">
              <button className="p-2 hover:bg-secondary rounded-full transition-colors text-muted-foreground hover:text-foreground">
                <Bell className="w-4 h-4" />
              </button>
              <button className="p-2 hover:bg-secondary rounded-full transition-colors text-muted-foreground hover:text-foreground">
                <HelpCircle className="w-4 h-4" />
              </button>
              <div className="w-8 h-8 rounded-full bg-accent text-accent-foreground grid place-items-center cursor-pointer ml-2">
                <User className="w-4 h-4" />
              </div>
            </div>
          </div>
        </header>

        {/* Content Area */}
        <main className="flex-1 overflow-y-auto p-8 relative">
          <div className="max-w-6xl mx-auto space-y-8 animate-fade-up">
            {title && (
              <div className="flex items-center justify-between">
                <h1 className="text-2xl font-bold tracking-tight text-foreground">{title}</h1>
              </div>
            )}
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}
