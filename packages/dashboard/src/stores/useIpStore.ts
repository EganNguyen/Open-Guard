import { create } from 'zustand';

interface IpEntry {
  ip: string;
  status: 'blocked' | 'allowlisted';
  reason?: string;
  addedAt: number;
}

interface IpState {
  blocked: IpEntry[];
  allowlisted: IpEntry[];
  loading: boolean;
  fetchIps: () => Promise<void>;
  blockIp: (ip: string, reason?: string) => Promise<void>;
  allowlistIp: (ip: string) => Promise<void>;
  removeIp: (ip: string) => Promise<void>;
}

export const useIpStore = create<IpState>((set, get) => ({
  blocked: [],
  allowlisted: [],
  loading: false,

  fetchIps: async () => {
    set({ loading: true });
    try {
      const response = await fetch('/api/guard/ips');
      const data = await response.json();
      set({
        blocked: data.blocked || [],
        allowlisted: data.allowlisted || [],
        loading: false,
      });
    } catch {
      set({ loading: false });
    }
  },

  blockIp: async (ip, reason) => {
    await fetch('/api/guard/ips/block', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip, reason }),
    });
    set((state) => ({
      blocked: [
        ...state.blocked,
        { ip, status: 'blocked', reason, addedAt: Date.now() },
      ],
    }));
  },

  allowlistIp: async (ip) => {
    await fetch('/api/guard/ips/allowlist', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip }),
    });
    set((state) => ({
      allowlisted: [
        ...state.allowlisted,
        { ip, status: 'allowlisted', addedAt: Date.now() },
      ],
    }));
  },

  removeIp: async (ip) => {
    await fetch(`/api/guard/ips/${encodeURIComponent(ip)}`, {
      method: 'DELETE',
    });
    set((state) => ({
      blocked: state.blocked.filter((e) => e.ip !== ip),
      allowlisted: state.allowlisted.filter((e) => e.ip !== ip),
    }));
  },
}));