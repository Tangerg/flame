import { type AccentSpec } from "@/features/theme/public/types";
import { DEFAULT_ACCENT } from "@/features/theme/kit/appearance";
import { definePlugin } from "@/extensions/definePlugin";
import { ACCENT } from "@/features/theme/public/extensionPoints";
const DEFAULT_ACCENTS: AccentSpec[] = [
  {
    id: "flame",
    label: "settings.accent.flame",
    color: DEFAULT_ACCENT,
    order: 0,
  },
  {
    id: "blue",
    label: "settings.accent.blue",
    color: "#0485f7",
    order: 1,
  },
  {
    id: "purple",
    label: "settings.accent.purple",
    color: "#7f52ff",
    order: 2,
  },
  {
    id: "green",
    label: "settings.accent.green",
    color: "#21a179",
    order: 3,
  },
];

export const defaultAccents = definePlugin({
  name: "flame.builtin.default-accents",
  setup(ctx) {
    for (const accent of DEFAULT_ACCENTS) {
      ctx.contributions.add(ACCENT, accent);
    }
  },
});
