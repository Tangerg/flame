import type { StoryCatalog } from "./catalog";
import {
  UI_FONT_SIZE_DEFAULT_PX,
  UI_FONT_SIZE_MIN_PX,
  UI_FONT_SIZE_MAX_PX,
  normalizeUiFontSize,
} from "@flame/ui/typography";
import type { LabLocale, LabMotion, LabStory, LabTheme } from "./story";

export const LAB_PATH = "/lab/";
export const LAB_INSTANCE_QUERY = "lab-instance";
const LAB_DEFAULT_LOCALE: LabLocale = "zh-CN";
export const FIXTURE_DEFAULT_LOCALE = "en";
export const LAB_FONT_SIZES = Array.from(
  { length: UI_FONT_SIZE_MAX_PX - UI_FONT_SIZE_MIN_PX + 1 },
  (_, index) => UI_FONT_SIZE_MIN_PX + index,
);
export const VIEWPORTS = [
  { id: "component", width: null },
  { id: "phone", width: 390 },
  { id: "tablet", width: 768 },
  { id: "desktop", width: 1280 },
] as const;

export type LabViewport = (typeof VIEWPORTS)[number]["id"];
type LabMode = "gallery" | "inspect";

export interface LabSelection {
  readonly storyId: string;
  readonly variantId: string;
  readonly theme: LabTheme;
  readonly locale: LabLocale;
  readonly motion: LabMotion;
  readonly fontSize: number;
  readonly viewport: LabViewport;
  readonly mode: LabMode;
  readonly compare: boolean;
}

export type FailureSelection = Pick<LabSelection, "theme" | "locale" | "motion" | "fontSize">;

export function fixtureTheme(value: string | null): LabTheme {
  return value === "dark" ? "dark" : "light";
}

export function fixtureColors(query: URLSearchParams) {
  const hex = (name: string) => {
    const value = query.get(name);
    return value !== null && /^#[\da-f]{6}$/i.test(value) ? value : undefined;
  };
  const accent = hex("accent");
  const contrastValue = query.get("contrast");
  const contrast =
    contrastValue !== null && Number.isFinite(Number(contrastValue))
      ? Math.min(100, Math.max(0, Number(contrastValue)))
      : undefined;
  const bg = hex("custom-bg");
  const fg = hex("custom-fg");
  return {
    ...(accent !== undefined ? { accent } : {}),
    ...(contrast !== undefined ? { contrast } : {}),
    ...(bg !== undefined && fg !== undefined
      ? { theme: "custom" as const, customTheme: { bg, fg } }
      : {}),
  };
}

export function failureSelection(
  query: URLSearchParams,
  context: "lab" | "fixture" = "lab",
): FailureSelection {
  const requestedLocale = query.get("locale");
  const locale =
    requestedLocale === "en"
      ? "en"
      : requestedLocale === "zh-CN" || requestedLocale === "zh"
        ? "zh-CN"
        : context === "fixture"
          ? FIXTURE_DEFAULT_LOCALE
          : LAB_DEFAULT_LOCALE;
  const requestedFont = query.get("font-size");
  const fontSize =
    context === "fixture" && requestedFont !== null && Number.isFinite(Number(requestedFont))
      ? normalizeUiFontSize(Number(requestedFont))
      : (LAB_FONT_SIZES.find((size) => String(size) === requestedFont) ?? UI_FONT_SIZE_DEFAULT_PX);
  return {
    theme: fixtureTheme(query.get("theme")),
    locale,
    motion: query.get("motion") === "full" ? "full" : "off",
    fontSize,
  };
}

export function recoveryURL(selection: FailureSelection): string {
  return `${LAB_PATH}?${new URLSearchParams({
    locale: selection.locale,
    theme: selection.theme,
    motion: selection.motion,
    "font-size": String(selection.fontSize),
  })}`;
}

function option<T extends string>(
  query: URLSearchParams,
  key: string,
  values: readonly T[],
  fallback: T,
): T {
  const value = query.get(key);
  if (value === null) return fallback;
  const found = values.find((candidate) => candidate === value);
  if (found === undefined) throw new Error(`Invalid ${key}: ${value}`);
  return found;
}

export function readSelection(search: string, catalog: StoryCatalog): LabSelection {
  const query = new URLSearchParams(search);
  const panel = option(query, "panel", ["foundation", "business"] as const, "foundation");
  const story = catalog.story(query.get("story") ?? catalog.first(panel).id);
  if (query.has("panel") && panel !== story.panel) {
    throw new Error(`Story does not belong to panel: ${story.id}`);
  }
  const variantId = query.get("variant") ?? story.variants[0].id;
  if (!story.variants.some((variant) => variant.id === variantId)) {
    throw new Error(`Unknown variant: ${story.id}/${variantId}`);
  }
  return {
    storyId: story.id,
    variantId,
    theme: option(query, "theme", ["light", "dark"] as const, "light"),
    locale: option(query, "locale", ["zh-CN", "en"] as const, LAB_DEFAULT_LOCALE),
    motion: option(query, "motion", ["off", "full"] as const, "off"),
    fontSize: Number(
      option(query, "font-size", LAB_FONT_SIZES.map(String), String(UI_FONT_SIZE_DEFAULT_PX)),
    ),
    viewport: option(
      query,
      "viewport",
      VIEWPORTS.map(({ id }) => id),
      "component",
    ),
    mode: option(query, "mode", ["gallery", "inspect"] as const, "gallery"),
    compare: option(query, "compare", ["off", "on"] as const, "off") === "on",
  };
}

export function selectStory(selection: LabSelection, story: LabStory): LabSelection {
  return { ...selection, storyId: story.id, variantId: story.variants[0].id };
}

export function selectionSearch(selection: LabSelection, catalog: StoryCatalog): string {
  return new URLSearchParams({
    panel: catalog.story(selection.storyId).panel,
    story: selection.storyId,
    variant: selection.variantId,
    theme: selection.theme,
    locale: selection.locale,
    motion: selection.motion,
    "font-size": String(selection.fontSize),
    viewport: selection.viewport,
    mode: selection.mode,
    compare: selection.compare ? "on" : "off",
  }).toString();
}

export function previewURL(story: LabStory, selection: LabSelection, instanceId: string): string {
  const query = new URLSearchParams({
    theme: selection.theme,
    locale: selection.locale,
    motion: selection.motion,
    "font-size": String(selection.fontSize),
    [LAB_INSTANCE_QUERY]: instanceId,
  });
  if (story.kind === "component") {
    query.set("story", story.id);
    query.set("variant", selection.variantId);
    return `${LAB_PATH}preview.html?${query}`;
  }
  const variant = story.variants.find(({ id }) => id === selection.variantId);
  if (!variant) throw new Error(`Unknown fixture variant: ${selection.variantId}`);
  query.set("locale", selection.locale === "zh-CN" ? "zh" : "en");
  query.set("fixture", variant.target.fixture);
  query.set("state", variant.target.state);
  if (variant.target.fixture === "workspace" && variant.target.pane) {
    query.set("pane", variant.target.pane);
  }
  return `${LAB_PATH}fixture.html?${query}`;
}

export function previewWidth(story: LabStory, viewport: LabViewport): number {
  return VIEWPORTS.find(({ id }) => id === viewport)!.width ?? story.width;
}
