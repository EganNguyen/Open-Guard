"use client";

import { useState, useEffect, useCallback } from "react";
import { Eye, EyeOff, Copy, Check, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";

interface ApiKeyRevealProps {
  apiKey: string;
}

export function ApiKeyReveal({ apiKey }: ApiKeyRevealProps) {
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);
  const [timer, setTimer] = useState(30);

  // Auto-mask after 30s
  useEffect(() => {
    if (!revealed) return;
    
    const interval = setInterval(() => {
      setTimer((prev) => {
        if (prev <= 1) {
          setRevealed(false);
          return 30;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [revealed]);

  // Reset timer when revealed
  useEffect(() => {
    if (revealed) setTimer(30);
  }, [revealed]);

  const handleCopy = () => {
    navigator.clipboard.writeText(apiKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-4 p-6 bg-secondary/30 border border-border rounded-xl animate-fade-up">
      <div className="flex items-center justify-between">
        <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-2">
          <ShieldAlert className="w-3.5 h-3.5 text-accent" />
          Client Secrets
        </label>
        {revealed && (
          <span className="text-[10px] font-mono text-amber flex items-center gap-1.5 animate-pulse">
            <Clock className="w-3 h-3" />
            Auto-masking in {timer}s
          </span>
        )}
      </div>

      <div className="relative group">
        <div className="flex items-center gap-2 bg-background border border-border rounded-lg p-3 transition-all focus-within:ring-2 focus-within:ring-accent/20">
          <input
            type={revealed ? "text" : "password"}
            readOnly
            value={apiKey}
            className="flex-1 bg-transparent border-none outline-none text-[13px] font-mono tracking-tight selection:bg-accent/30"
          />
          <div className="flex items-center gap-1 border-l border-border pl-2">
            <button
              onClick={() => setRevealed(!revealed)}
              className="p-1.5 hover:bg-secondary rounded-md transition-colors text-muted-foreground hover:text-foreground"
              title={revealed ? "Hide key" : "Reveal key"}
            >
              {revealed ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
            <button
              onClick={handleCopy}
              className="p-1.5 hover:bg-secondary rounded-md transition-colors text-muted-foreground hover:text-foreground"
              title="Copy to clipboard"
            >
              {copied ? <Check className="w-4 h-4 text-green" /> : <Copy className="w-4 h-4" />}
            </button>
          </div>
        </div>
      </div>

      <div className="flex gap-3 bg-amber/5 border border-amber/20 rounded-lg p-3">
        <AlertTriangle className="w-4 h-4 text-amber shrink-0 mt-0.5" />
        <p className="text-[11px] text-amber leading-relaxed">
          <strong>Security Warning:</strong> This key grants full programmatic access to your 
          organization. Store it in a secure vault (e.g., Vault, AWS Secrets Manager). 
          It will <strong>never</strong> be shown again once you leave this page.
        </p>
      </div>
    </div>
  );
}

// Sub-components for Icons
function ShieldAlert(props: any) {
  return (
    <svg {...props} xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg>
  );
}

function Clock(props: any) {
  return (
    <svg {...props} xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
  );
}
