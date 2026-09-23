import { create } from "zustand";

interface SessionSearchState {
  open: boolean;
  setOpen: (open: boolean) => void;
  show: () => void;
  toggle: () => void;
}

export const useSessionSearchStore = create<SessionSearchState>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  show: () => set({ open: true }),
  toggle: () => set((state) => ({ open: !state.open })),
}));
