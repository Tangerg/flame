import type { AnyPlugin } from "dougong";
import { defineLocale } from "./defineLocale";

const BUNDLED = [
  { id: "en", label: "English" },
  { id: "zh", label: "简体中文", load: () => import("@/lib/i18n/locales/zh").then((m) => m.zh) },
  {
    id: "zh-TW",
    label: "繁體中文",
    load: () => import("@/lib/i18n/locales/zh-TW").then((m) => m.zhTW),
  },
  { id: "ja", label: "日本語", load: () => import("@/lib/i18n/locales/ja").then((m) => m.ja) },
  { id: "ko", label: "한국어", load: () => import("@/lib/i18n/locales/ko").then((m) => m.ko) },
  { id: "es", label: "Español", load: () => import("@/lib/i18n/locales/es").then((m) => m.es) },
  { id: "fr", label: "Français", load: () => import("@/lib/i18n/locales/fr").then((m) => m.fr) },
  { id: "de", label: "Deutsch", load: () => import("@/lib/i18n/locales/de").then((m) => m.de) },
];

export const localePlugins: AnyPlugin[] = BUNDLED.map((locale, index) =>
  defineLocale({ ...locale, order: index * 10 }),
);
