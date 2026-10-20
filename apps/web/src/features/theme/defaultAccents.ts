import { type AccentSpec } from "@/features/theme/public/types";
import { DEFAULT_ACCENT } from "@/features/theme/kit/appearance";
import { definePlugin } from "@/extensions/definePlugin";
import { ACCENT } from "@/features/theme/public/extensionPoints";
export const DEFAULT_ACCENTS: AccentSpec[] = [
  {
    id: "flame",
    label: "Flame",
    color: DEFAULT_ACCENT,
    order: 0,
  },
  {
    id: "blue",
    label: "Blue",
    color: "#0485f7",
    order: 1,
  },
  {
    id: "purple",
    label: "Purple",
    color: "#7f52ff",
    order: 2,
  },
  {
    id: "green",
    label: "Green",
    color: "#21a179",
    order: 3,
  },
];

export const defaultAccents = definePlugin({
  name: "flame.builtin.default-accents",
  setup(ctx) {
    for (const accent of DEFAULT_ACCENTS) {
      ctx.contribute(ACCENT, accent);
    }
  },
});
