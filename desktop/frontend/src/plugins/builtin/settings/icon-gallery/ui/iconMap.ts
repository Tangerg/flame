import * as Glyphs from "@flame/icons/react";
import type { IconComponent } from "@flame/icons/react";
import { icons } from "@flame/icons/catalog";

export const IconMap: Readonly<Record<string, IconComponent>> = Glyphs;
export { icons };
export type CatalogEntry = (typeof icons)[number];
export const IconByName: Readonly<Record<string, CatalogEntry>> = Object.fromEntries(
  icons.map((entry) => [entry.name, entry]),
);
