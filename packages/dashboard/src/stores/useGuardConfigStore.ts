import { create } from 'zustand';
import type { DetectorConfig, GuardMode } from '@open-guard/core';

interface GuardConfigState {
  detectors: DetectorConfig[];
  mode: GuardMode;
  dirty: boolean;
  setDetectors: (detectors: DetectorConfig[]) => void;
  setDetectorEnabled: (id: string, enabled: boolean) => void;
  updateDetectorOptions: (id: string, options: Record<string, unknown>) => void;
  setMode: (mode: GuardMode) => void;
  resetToSaved: () => void;
  save: () => Promise<void>;
  fetchConfig: () => Promise<void>;
}

const initialDetectors: DetectorConfig[] = [];

export const useGuardConfigStore = create<GuardConfigState>((set, get) => ({
  detectors: initialDetectors,
  mode: 'enforce',
  dirty: false,

  setDetectors: (detectors) => set({ detectors, dirty: true }),

  setDetectorEnabled: (id, enabled) => set((state) => ({
    detectors: state.detectors.map(d =>
      d.id === id ? { ...d, enabled } : d
    ),
    dirty: true,
  })),

  updateDetectorOptions: (id, options) => set((state) => ({
    detectors: state.detectors.map(d =>
      d.id === id ? { ...d, options: { ...d.options, ...options } } : d
    ),
    dirty: true,
  })),

  setMode: (mode) => set({ mode, dirty: true }),

  resetToSaved: () => set({ dirty: false }),

  save: async () => {
    const { detectors, mode } = get();
    try {
      await fetch('/api/guard/config', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ detectors, mode }),
      });
      set({ dirty: false });
    } catch (error) {
      console.error('Failed to save config:', error);
      throw error;
    }
  },

  fetchConfig: async () => {
    try {
      const response = await fetch('/api/guard/config');
      const data = await response.json();
      set({
        detectors: data.detectors || [],
        mode: data.mode || 'enforce',
        dirty: false,
      });
    } catch (error) {
      console.error('Failed to fetch config:', error);
    }
  },
}));