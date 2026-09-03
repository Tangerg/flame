import type { AnyPlugin } from "dougong";
import { defineVisualStylePlugin } from "./defineVisualStylePlugin";
import { flameStyle } from "./flame";

const builtinVisualStyleSpecs = [flameStyle] as const;

export const builtinVisualStyles: AnyPlugin[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
