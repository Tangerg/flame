// A working dot, an unread count and a base title are each set by a different plugin, so
// one composer owns the string and each setter owns one field — no claim can erase another.
// Its own store rather than a registry slice: this is window state, not registration.

import { create } from "zustand";
import { PRODUCT_NAME } from "@/product";

interface WindowState {
  title: string;
  badge: number;
  working: boolean;
  setTitle: (text: string) => void;
  setBadge: (n: number) => void;
  setWorking: (on: boolean) => void;
}

function compose(base: string, badge: number, working: boolean): void {
  if (typeof document === "undefined") return;
  const dot = working ? "● " : "";
  const count = badge > 0 ? `(${badge}) ` : "";
  document.title = `${dot}${count}${base || PRODUCT_NAME}`;
}

export const useWindowStore = create<WindowState>((set, get) => ({
  title: "",
  badge: 0,
  working: false,
  setTitle(text) {
    set({ title: text });
    compose(text, get().badge, get().working);
  },
  setBadge(n) {
    set({ badge: n });
    compose(get().title, n, get().working);
  },
  setWorking(on) {
    set({ working: on });
    compose(get().title, get().badge, on);
  },
}));
