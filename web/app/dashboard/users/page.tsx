"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Shell from "@/components/layout/Shell";
import { 
  getUsers, 
  createUser, 
  suspendUser, 
  activateUser, 
  deleteUser,
  User 
} from "@/lib/api";
import { 
  Plus, 
  Search, 
  MoreVertical, 
  UserPlus, 
  ShieldCheck, 
  ShieldAlert, 
  Mail, 
  Calendar, 
  UserMinus,
  CheckCircle2,
  AlertCircle
} from "lucide-react";
import { cn, formatDate } from "@/lib/utils";

export default function UsersPage() {
  const queryClient = useQueryClient();
  const [showAddUser, setShowAddUser] = useState(false);
  
  // Form State
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");

  const { data: usersRes, status } = useQuery({
    queryKey: ["users"],
    queryFn: () => getUsers(),
  });

  const createMutation = useMutation({
    mutationFn: (data: any) => createUser(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setShowAddUser(false);
      setEmail("");
      setDisplayName("");
    },
  });

  const suspendMutation = useMutation({
    mutationFn: (id: string) => suspendUser(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["users"] }),
  });

  const activateMutation = useMutation({
    mutationFn: (id: string) => activateUser(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["users"] }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["users"] }),
  });

  const users = usersRes?.data ?? [];

  return (
    <Shell 
      title="Managed Identities" 
      crumbs={["Identity", "Users"]}
    >
      <div className="space-y-6 animate-fade-up">
        {/* Header Summary */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <SummaryItem label="Total Users" value={users.length.toString()} />
            <SummaryItem label="MFA Coverage" value={`${Math.round((users.filter(u => u.mfa_enabled).length / (users.length || 1)) * 100)}%`} />
            <SummaryItem label="Active Sessions" value="23" />
          </div>
          
          <button 
            className="btn btn-primary h-10 px-6 gap-2"
            onClick={() => setShowAddUser(true)}
            data-testid="add-user-btn"
          >
            <UserPlus className="w-4 h-4" />
            Add new identity
          </button>
        </div>

        {/* Filters & Actions */}
        <div className="flex items-center gap-4">
          <div className="relative group flex-1 max-w-md">
            <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2 group-focus-within:text-accent transition-colors" />
            <input 
              type="text" 
              placeholder="Search by name, email, or role..." 
              className="bg-surface-1 border border-border rounded-lg pl-10 pr-4 py-2 text-[13px] w-full focus:outline-none focus:ring-1 focus:ring-accent transition-all"
            />
          </div>
          
          <div className="flex items-center gap-2">
            <button className="btn btn-ghost text-[12px] h-9">Export CSV</button>
            <button className="btn btn-ghost text-[12px] h-9">Bulk Actions</button>
          </div>
        </div>

        {/* Users Table */}
        <div className="bg-surface-1 border border-border rounded-xl overflow-hidden shadow-sm">
          <table className="w-full text-left border-collapse">
            <thead className="bg-secondary/50 border-b border-border">
              <tr className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                <th className="px-6 py-4">Identity</th>
                <th className="px-6 py-4">Status</th>
                <th className="px-6 py-4">MFA Policy</th>
                <th className="px-6 py-4">Last Activity</th>
                <th className="px-4 py-4 w-12"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border" data-testid="user-list">
              {status === "pending" ? (
                Array.from({ length: 4 }).map((_, i) => (
                  <tr key={i} className="animate-pulse">
                    <td colSpan={5} className="px-6 py-4 h-16 bg-secondary/10" />
                  </tr>
                ))
              ) : users.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-6 py-20 text-center space-y-3">
                    <div className="w-12 h-12 rounded-full bg-secondary grid place-items-center mx-auto mb-4">
                      <UserPlus className="w-6 h-6 text-muted-foreground" />
                    </div>
                    <h3 className="font-bold tracking-tight">No identities found</h3>
                    <p className="text-sm text-muted-foreground max-w-sm mx-auto leading-relaxed">
                      Identity management is centralized. Add users to your organization 
                      to begin enforcing MFA and RBAC policies.
                    </p>
                  </td>
                </tr>
              ) : (
                users.map((user) => (
                  <tr 
                    key={user.id} 
                    className="group hover:bg-secondary/30 transition-colors"
                  >
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 transition-all flex items-center justify-center font-bold text-accent text-[12px] group-hover:scale-110">
                          {user.display_name?.substring(0, 1) || user.email.substring(0, 1).toUpperCase()}
                        </div>
                        <div className="space-y-0.5">
                          <span className="text-[13px] font-bold tracking-tight block">
                            {user.display_name || "New Identity"}
                          </span>
                          <span className="text-[11px] text-muted-foreground flex items-center gap-1.5">
                            <Mail className="w-3 h-3" />
                            {user.email}
                          </span>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <span className={cn(
                        "tag capitalize",
                        user.status === "active" ? "tag-green" : "tag-red"
                      )}>
                        {user.status}
                      </span>
                    </td>
                    <td className="px-6 py-4">
                      {user.mfa_enabled ? (
                        <div className="flex items-center gap-2 text-green font-medium text-[11px]">
                          <CheckCircle2 className="w-3.5 h-3.5" />
                          Enforced
                        </div>
                      ) : (
                        <div className="flex items-center gap-2 text-amber font-medium text-[11px]">
                          <AlertCircle className="w-3.5 h-3.5" />
                          Unprotected
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-4 tabular-nums">
                      <div className="flex flex-col text-[12px] font-medium">
                        <span className="flex items-center gap-1.5 text-foreground">
                          <Calendar className="w-3.5 h-3.5 text-muted-foreground" />
                          {formatDate(user.created_at)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-4 text-right">
                      <div className="flex items-center justify-end gap-1">
                        {user.status === "active" ? (
                          <button 
                            onClick={() => suspendMutation.mutate(user.id)}
                            className="p-2 hover:bg-destructive/10 rounded-lg transition-colors text-muted-foreground hover:text-destructive"
                            title="Suspend Access"
                          >
                            <ShieldAlert className="w-4 h-4" />
                          </button>
                        ) : (
                          <button 
                            onClick={() => activateMutation.mutate(user.id)}
                            className="p-2 hover:bg-green/10 rounded-lg transition-colors text-muted-foreground hover:text-green"
                            title="Activate Access"
                          >
                            <CheckCircle2 className="w-4 h-4" />
                          </button>
                        )}
                        <button 
                          onClick={() => deleteMutation.mutate(user.id)}
                          className="p-2 hover:bg-destructive/10 rounded-lg transition-colors text-muted-foreground hover:text-destructive"
                        >
                          <UserMinus className="w-4 h-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Add User Modal */}
      {showAddUser && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-background/90 backdrop-blur-md animate-in fade-in duration-200">
          <div className="w-full max-w-md bg-surface-1 border border-border rounded-xl shadow-2xl overflow-hidden animate-in zoom-in-95 duration-200">
            <div className="p-6 border-b border-border bg-secondary/30">
              <h3 className="text-lg font-bold tracking-tight">Provision Global Identity</h3>
              <p className="text-xs text-muted-foreground mt-1 leading-relaxed">
                Add an identity to the central control plane. Identities can then 
                be managed, audited, and enforced via global security policies.
              </p>
            </div>
            <form 
              onSubmit={(e) => {
                e.preventDefault();
                createMutation.mutate({ email, display_name: displayName });
              }} 
              className="p-6 space-y-6"
            >
              <div className="space-y-4">
                <div className="space-y-1.5 font-bold tracking-tight text-foreground">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Identity Endpoint (Email)</label>
                  <input 
                    type="email" 
                    required 
                    placeholder="user@acme.com"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    data-testid="create-user-email"
                    className="w-full bg-secondary/50 border border-border rounded-lg px-4 py-3 text-[13px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent"
                  />
                </div>
                <div className="space-y-1.5 font-bold tracking-tight text-foreground">
                  <label className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground ml-1">Professional Display Name</label>
                  <input 
                    type="text" 
                    placeholder="Jane Smith"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    data-testid="create-user-display-name"
                    className="w-full bg-secondary/50 border border-border rounded-lg px-4 py-3 text-[13px] focus:outline-none focus:ring-2 focus:ring-accent/20 focus:border-accent"
                  />
                </div>
              </div>

              <div className="flex items-center gap-3 pt-2">
                <button 
                  type="button" 
                  onClick={() => setShowAddUser(false)}
                  className="flex-1 btn btn-ghost py-3 font-bold"
                >
                  Cancel
                </button>
                <button 
                  type="submit" 
                  disabled={createMutation.isPending}
                  className="flex-[2] btn btn-primary py-3 font-bold"
                  data-testid="submit-create-user"
                >
                  {createMutation.isPending ? "Signing Identity..." : "Commit Identity"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </Shell>
  );
}

function SummaryItem({ label, value, status }: any) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">{label}</span>
      <div className="flex items-center gap-2">
        <span className="text-[18px] font-bold tabular-nums tracking-tight">{value}</span>
        {status === "healthy" && <div className="w-1.5 h-1.5 rounded-full bg-green animate-pulse" />}
      </div>
    </div>
  );
}
