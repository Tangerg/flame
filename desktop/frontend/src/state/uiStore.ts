import { z } from "zod";
import { ACCENT_TINTS, DEFAULT_ACCENT_TINT, type AccentTint } from "@/lib/appearance";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { DEFAULT_UI_DENSITY, UI_DENSITY_MODES, type UiDensity } from "@/lib/density";
import type { ColorThemeId, VisualStyleId } from "@/lib/appearance";
import { discardOlderVersions } from "@/lib/persistedStore";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
// Direct registry import: the SDK barrel pulls in host.ts, which imports this file, and the
// cycle is a TDZ error under Vitest. Same reason for the deep extension-point paths below.
import type { CustomTheme, UiState } from "./uiPreferences";

export type { CustomTheme, UiState } from "./uiPreferences";

// Read back as COLOURS, not opaque strings. `parseInt(hex, 16)` does not reject a non-hex
// value, it reads whatever prefix parses — "blue" returns a finite garbage colour and every
// derived surface paints black. Rejecting here is what makes a corrupt payload boot clean.
const HEX_COLOUR = z.string().regex(/^#[0-9a-fA-F]{6}$/);

const uiPersistSchema = z.object({
  theme: z.string(),
  visualStyle: z.string(),
  accent: HEX_COLOUR,
  customTheme: z.object({ bg: HEX_COLOUR, fg: HEX_COLOUR }),
  contrast: z.number(),
  accentTint: z.enum(ACCENT_TINTS),
  uiFont: z.string(),
  codeFont: z.string(),
  fontSize: z.number().nullable(),
  fontSmoothing: z.boolean(),
  density: z.enum(UI_DENSITY_MODES),
  radiusScale: z.number(),
  motionScale: z.number(),
  streamReveal: z.enum(["smooth", "typewriter"]),
  sidebarCollapsed: z.boolean(),
  sidebarWidth: z.number(),
  dockWidthRatio: z.number().min(0).max(1).nullable(),
  completionSound: z.boolean(),
});

interface UiActions {
  setTheme: (theme: ColorThemeId) => void;
  setVisualStyle: (visualStyle: VisualStyleId) => void;
  /** Flips to the opposite SCHEME, not the "dark"/"light" id, so custom themes still
   *  toggle. No-op when no theme of the opposite scheme is registered. */
  setAccent: (accent: string) => void;
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
  setContrast: (contrast: number) => void;
  setAccentTint: (accentTint: AccentTint) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setDensity: (density: UiDensity) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
  setStreamReveal: (mode: "smooth" | "typewriter") => void;
  toggleSidebar: () => void;
  setSidebarWidth: (width: number) => void;
  setDockWidthRatio: (ratio: number | null) => void;
  setCompletionSound: (on: boolean) => void;
}

export const useUiStore = create<UiState & UiActions>()(
  persist(
    (set) => ({
      theme: "light",
      visualStyle: "flame",
      accent: "#3574f0",
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
      contrast: 25,
      accentTint: DEFAULT_ACCENT_TINT,
      uiFont: "",
      codeFont: "",
      fontSize: null,
      fontSmoothing: true,
      density: DEFAULT_UI_DENSITY,
      radiusScale: 1,
      motionScale: 1,
      streamReveal: "smooth",
      sidebarCollapsed: false,
      sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
      dockWidthRatio: null,
      completionSound: false,

      setTheme: (theme) => set({ theme }),
      setVisualStyle: (visualStyle) => set({ visualStyle }),
      setAccent: (accent) => set({ accent }),
      setCustomTheme: (patch) => set((s) => ({ customTheme: { ...s.customTheme, ...patch } })),
      setContrast: (contrast) => set({ contrast }),
      setAccentTint: (accentTint) => set({ accentTint }),
      setUiFont: (uiFont) => set({ uiFont }),
      setCodeFont: (codeFont) => set({ codeFont }),
      setFontSize: (fontSize) => set({ fontSize }),
      setFontSmoothing: (fontSmoothing) => set({ fontSmoothing }),
      setDensity: (density) => set({ density }),
      setRadiusScale: (radiusScale) => set({ radiusScale }),
      setMotionScale: (motionScale) => set({ motionScale }),
      setStreamReveal: (streamReveal) => set({ streamReveal }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarWidth: (sidebarWidth) => set({ sidebarWidth }),
      setDockWidthRatio: (dockWidthRatio) => set({ dockWidthRatio }),
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: "flame.ui",
      storage: createJSONStorage(() => localStorage),
      version: 14,
      migrate: discardOlderVersions,
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = uiPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn("[uiStore] discarding corrupted flame.ui:", parsed.error.issues);
          return current;
        }
        return { ...current, ...parsed.data };
      },
    },
  ),
);
