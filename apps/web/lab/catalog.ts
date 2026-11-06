import { foundationStories } from "./foundationCatalog";
import { businessStories } from "./businessCatalog";
import { localize, type LabLocale, type LabPanel, type LabStory, type LabVariant } from "./story";

const IDENTIFIER = /^[a-z][a-z0-9-]*$/;

function ownVariants<V extends LabVariant>(
  variants: readonly [V, ...V[]],
  copy: (variant: V) => V,
): readonly [V, ...V[]] {
  const owned: [V, ...V[]] = [copy(variants[0]), ...variants.slice(1).map(copy)];
  return Object.freeze(owned);
}

function ownStory(story: LabStory): LabStory {
  const description = {
    title: Object.freeze({ ...story.title }),
    summary: Object.freeze({ ...story.summary }),
    group: Object.freeze({ ...story.group }),
  };
  if (story.kind === "fixture") {
    return Object.freeze({
      ...story,
      ...description,
      variants: ownVariants(story.variants, (variant) =>
        Object.freeze({
          ...variant,
          title: Object.freeze({ ...variant.title }),
          target: Object.freeze({ ...variant.target }),
        }),
      ),
    });
  }
  return Object.freeze({
    ...story,
    ...description,
    variants: ownVariants(story.variants, (variant) =>
      Object.freeze({
        ...variant,
        title: Object.freeze({ ...variant.title }),
      }),
    ),
  });
}

export class StoryCatalog {
  readonly stories: readonly LabStory[];

  constructor(stories: readonly LabStory[]) {
    const ids = new Set<string>();
    for (const story of stories) {
      if (!IDENTIFIER.test(story.id) || ids.has(story.id)) {
        throw new Error(`Invalid or duplicate story: ${story.id}`);
      }
      ids.add(story.id);
      if (!story.variants.length) throw new Error(`Story has no variants: ${story.id}`);
      const variants = new Set<string>();
      for (const variant of story.variants) {
        if (!IDENTIFIER.test(variant.id) || variants.has(variant.id)) {
          throw new Error(`Invalid or duplicate variant: ${story.id}/${variant.id}`);
        }
        variants.add(variant.id);
      }
      if (![story.width, story.height].every((value) => Number.isSafeInteger(value) && value > 0)) {
        throw new Error(`Invalid preview dimensions: ${story.id}`);
      }
      if (!/^[\w.-]+(?:\/[\w.-]+)+$/.test(story.source) || story.source.includes("..")) {
        throw new Error(`Invalid source path: ${story.id}`);
      }
    }
    this.stories = Object.freeze(stories.map(ownStory));
    for (const panel of ["foundation", "business"] as const) {
      if (!stories.some((story) => story.panel === panel)) {
        throw new Error(`Empty component panel: ${panel}`);
      }
    }
  }

  story(id: string): LabStory {
    const story = this.stories.find((candidate) => candidate.id === id);
    if (!story) throw new Error(`Unknown story: ${id}`);
    return story;
  }

  first(panel: LabPanel): LabStory {
    return this.stories.find((story) => story.panel === panel)!;
  }

  search(panel: LabPanel, query: string, locale: LabLocale): readonly LabStory[] {
    const words = query.toLocaleLowerCase(locale).trim().split(/\s+/);
    return this.stories.filter((story) => {
      if (story.panel !== panel) return false;
      const searchable = [
        story.id,
        story.source,
        localize(story.title, locale),
        localize(story.group, locale),
        localize(story.summary, locale),
        ...story.variants.map((variant) => `${variant.id} ${localize(variant.title, locale)}`),
      ]
        .join(" ")
        .toLocaleLowerCase(locale);
      return words.every((word) => searchable.includes(word));
    });
  }
}

export const catalog = new StoryCatalog([...foundationStories, ...businessStories]);
