import { create } from 'zustand';

interface DraftState {
  hasChanges: boolean;
  runningVersion: number;
  setStatus: (hasChanges: boolean, runningVersion: number) => void;
}

export const useDraftStore = create<DraftState>((set) => ({
  hasChanges: false,
  runningVersion: 0,
  setStatus: (hasChanges, runningVersion) => set({ hasChanges, runningVersion }),
}));
