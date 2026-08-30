import type { ComponentType } from "react";
import { toc as rawToc } from "@lobehub/icons/es/toc";

const monoModules = import.meta.glob<{ default: ComponentType<{ size?: number }> }>(
  "../../../node_modules/@lobehub/icons/es/*/components/Mono.js",
  { eager: true },
);

export const IconMap: Record<string, ComponentType<{ size?: number }>> = {};
for (const [path, mod] of Object.entries(monoModules)) {
  const match = path.match(/\/es\/([^/]+)\/components\/Mono\.js$/);
  if (match) IconMap[match[1]!] = mod.default;
}

export { rawToc };
export type TocEntry = (typeof rawToc)[number];

export const TocById: Record<string, TocEntry> = {};
for (const e of rawToc) TocById[e.id] = e;
