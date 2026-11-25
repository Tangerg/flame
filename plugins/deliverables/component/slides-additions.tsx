import { useState } from "react";
import { type Translate } from "@flame/component-sdk";
import { ActionMenu } from "@flame/ui";
import { ADDED_SHAPES, type Frame } from "../contracts/content";
import type { SlideLayout, SlideView } from "../contracts/layouts";
import type { Revision } from "./types";

type Insert = (operations: unknown[], within?: SlideView) => void;

// What an insertion added is selected once the editor shows the revision it
// produced: the slide, or the element on the slide it was added to.
export function useInsertion(
  live: {
    viewed: string;
    restructure(operations: unknown[]): Promise<Revision | undefined> | void;
  },
  saved: SlideLayout | undefined,
  select: (slide: string, element?: string) => void,
): Insert {
  const [inserted, setInserted] = useState<{
    revision: string;
    slide?: string;
    known: ReadonlySet<string>;
  }>();
  if (inserted && saved && live.viewed === inserted.revision) {
    setInserted(undefined);
    const ids = inserted.slide
      ? saved.slides
          .find((entry) => entry.id === inserted.slide)
          ?.elements.map((entry) => entry.id)
      : saved.slides.map((entry) => entry.id);
    const added = ids?.find((id) => !inserted.known.has(id));
    if (added && inserted.slide) select(inserted.slide, added);
    else if (added) select(added);
  }
  return (operations, within) => {
    if (!saved) return;
    const known = new Set(
      (within ? within.elements : saved.slides).map((entry) => entry.id),
    );
    void live.restructure(operations)?.then((revision) => {
      if (revision)
        setInserted({ revision: revision.id, slide: within?.id, known });
    });
  };
}

function centered(layout: SlideLayout, width: number, height: number): Frame {
  const w = Math.round(layout.width * width);
  const h = Math.round(layout.height * height);
  return {
    x: Math.round((layout.width - w) / 2),
    y: Math.round((layout.height - h) / 2),
    width: w,
    height: h,
  };
}

export function SlideAdditions({
  layout,
  slide,
  disabled,
  onInsert,
  t,
}: {
  layout: SlideLayout;
  slide: SlideView;
  disabled: boolean;
  onInsert: Insert;
  t: Translate;
}) {
  return (
    <>
      <ActionMenu
        label={t("slides.insertSlide")}
        triggerContent={t("slides.insertSlide")}
        disabled={disabled}
        items={layout.layouts.map((entry, index) => ({
          label: entry.name || t("slides.layout", { number: index + 1 }),
          onSelect: () =>
            onInsert([
              { type: "insertSlide", after: slide.id, layout: entry.id },
            ]),
        }))}
      />
      <ActionMenu
        label={t("slides.addElement")}
        triggerContent={t("slides.addElement")}
        disabled={disabled}
        items={[
          {
            label: t("slides.add.text"),
            onSelect: () =>
              onInsert(
                [
                  {
                    type: "addText",
                    slide: slide.id,
                    frame: centered(layout, 0.5, 0.15),
                    text: t("slides.newText"),
                  },
                ],
                slide,
              ),
          },
          ...ADDED_SHAPES.map((shape) => ({
            label: t(`slides.add.${shape}`),
            onSelect: () =>
              onInsert(
                [
                  {
                    type: "addShape",
                    slide: slide.id,
                    frame: centered(layout, 0.25, 0.25),
                    shape,
                  },
                ],
                slide,
              ),
          })),
        ]}
      />
    </>
  );
}
