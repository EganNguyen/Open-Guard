import { create } from 'zustand';
import type { GuardStreamEvent } from '@open-guard/sdk';

interface LiveFeedState {
  events: GuardStreamEvent[];
  paused: boolean;
  push: (event: GuardStreamEvent) => void;
  clear: () => void;
  togglePause: () => void;
}

export const useLiveFeedStore = create<LiveFeedState>((set) => ({
  events: [],
  paused: false,

  push: (event) => set((state) => ({
    events: state.paused ? state.events : [event, ...state.events].slice(0, 50),
  })),

  clear: () => set({ events: [] }),

  togglePause: () => set((state) => ({ paused: !state.paused })),
}));