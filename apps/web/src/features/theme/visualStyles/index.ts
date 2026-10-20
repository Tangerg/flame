import { type AnyPlugin } from "dougong";
import { defineVisualStylePlugin } from "@/features/theme/visualStyles/defineVisualStylePlugin";
import { flameStyle } from "@/features/theme/visualStyles/flame";
const builtinVisualStyleSpecs = [flameStyle] as const;

export const builtinVisualStyles: AnyPlugin[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
