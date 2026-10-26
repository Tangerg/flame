import { z } from "zod";
import { createStore } from "zustand/vanilla";
import { createJSONStorage, persist } from "zustand/middleware";
import { rehydrateOrDefault } from "@/lib/persistedStore";
import { type Paired } from "@/lib/persistedStore";
import {
  DEFAULT_ACCENT,
  DEFAULT_CONTRAST,
  DEFAULT_PAPER_TEXTURE_STRENGTH,
  DEFAULT_UI_DENSITY,
  PAPER_TEXTURES,
  UI_DENSITY_MODES,
  type AppearanceEdit,
  type AppearancePreference,
} from "@/features/theme/kit/appearance";
const APPEARANCE_STORAGE_KEY = "flame.appearance";

const HEX_COLOUR = z.string().regex(/^#[0-9a-fA-F]{6}$/);

const appearancePersistSchema = z.object({
  theme: z.string(),
  visualStyle: z.string(),
  accent: HEX_COLOUR,
  customTheme: z.object({ bg: HEX_COLOUR, fg: HEX_COLOUR }),
  contrast: z.number(),
  uiFont: z.string(),
  codeFont: z.string(),
  fontSize: z.number().nullable(),
  codeFontSize: z.number().nullable().default(null),
  fontSmoothing: z.boolean(),
  density: z.enum(UI_DENSITY_MODES),
  radiusScale: z.number(),
  motionScale: z.number(),
  paperTexture: z.enum(PAPER_TEXTURES).default("off"),
  paperTextureStrength: z.number().int().min(1).max(5).default(DEFAULT_PAPER_TEXTURE_STRENGTH),
});

const _paired: Paired<AppearancePreference, z.infer<typeof appearancePersistSchema>> = true;

void _paired;

export function createAppearanceStore(storage: () => Storage) {
  return createStore<AppearancePreference & AppearanceEdit>()(
    persist(
      (set) => ({
        theme: "system",
        visualStyle: "flame",
        accent: DEFAULT_ACCENT,
        customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
        contrast: DEFAULT_CONTRAST,
        uiFont: "",
        codeFont: "",
        fontSize: null,
        codeFontSize: null,
        fontSmoothing: true,
        density: DEFAULT_UI_DENSITY,
        radiusScale: 1,
        motionScale: 1,
        paperTexture: "off",
        paperTextureStrength: DEFAULT_PAPER_TEXTURE_STRENGTH,

        setTheme: (theme) => set({ theme }),
        setVisualStyle: (visualStyle) => set({ visualStyle }),
        setAccent: (accent) => set({ accent }),
        setCustomTheme: (patch) => set((s) => ({ customTheme: { ...s.customTheme, ...patch } })),
        setContrast: (contrast) => set({ contrast }),
        setUiFont: (uiFont) => set({ uiFont }),
        setCodeFont: (codeFont) => set({ codeFont }),
        setFontSize: (fontSize) => set({ fontSize }),
        setCodeFontSize: (codeFontSize) => set({ codeFontSize }),
        setFontSmoothing: (fontSmoothing) => set({ fontSmoothing }),
        setDensity: (density) => set({ density }),
        setRadiusScale: (radiusScale) => set({ radiusScale }),
        setMotionScale: (motionScale) => set({ motionScale }),
        setPaperTexture: (paperTexture) => set({ paperTexture }),
        setPaperTextureStrength: (paperTextureStrength) => set({ paperTextureStrength }),
      }),
      {
        name: APPEARANCE_STORAGE_KEY,
        storage: createJSONStorage(storage),
        merge: rehydrateOrDefault(APPEARANCE_STORAGE_KEY, appearancePersistSchema),
      },
    ),
  );
}
