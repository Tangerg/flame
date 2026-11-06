import type { StoryCatalog } from "./catalog";
import { previewURL, previewWidth, type LabSelection } from "./selection";

export interface LabCase {
  readonly id: string;
  readonly storyId: string;
  readonly variantId: string;
  readonly kind: "component" | "fixture";
  readonly width: number;
  readonly height: number;
  readonly url: string;
}

// This is a projection of the catalog, shared by the lab and its browser verifier.
export function labCases(catalog: StoryCatalog, selection: LabSelection): readonly LabCase[] {
  return catalog.stories.flatMap((story) =>
    story.variants.map((variant) => ({
      id: `${story.id}--${variant.id}`,
      storyId: story.id,
      variantId: variant.id,
      kind: story.kind,
      width: previewWidth(story, selection.viewport),
      height: story.height,
      url: previewURL(story, { ...selection, variantId: variant.id }, `verify:${story.id}`),
    })),
  );
}
