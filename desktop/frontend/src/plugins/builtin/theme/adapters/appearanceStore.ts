import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import {
  ACCENT_TINTS,
  DEFAULT_ACCENT_TINT,
  DEFAULT_UI_DENSITY,
  UI_DENSITY_MODES,
  type AppearanceEdit,
  type AppearancePreference,
} from "../kit/appearance";

// Read back as COLOURS, not opaque strings. `parseInt(hex, 16)` does not reject a non-hex
// value, it reads whatever prefix parses — "blue" returns a finite garbage colour and every
// derived surface paints black. Rejecting here is what makes a corrupt payload boot clean.
const APPEARANCE_STORAGE_KEY = "flame.appearance";

const HEX_COLOUR = z.string().regex(/^#[0-9a-fA-F]{6}$/);

const appearancePersistSchema = z.object({
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
});

export const useAppearanceStore = create<AppearancePreference & AppearanceEdit>()(
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
    }),
    {
      name: APPEARANCE_STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: rehydrateOrDefault(APPEARANCE_STORAGE_KEY, appearancePersistSchema),
    },
  ),
);
