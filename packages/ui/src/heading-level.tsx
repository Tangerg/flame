import { createContext, use, type ReactNode } from "react";

// Headings in these primitives derive their level from context so a nested
// layout produces a correct, non-skipping outline instead of hardcoded tags.
// The default is 2: the surrounding app shell owns the sole <h1>, and a Panel's
// title sits one level below it. A region that is itself the top of its own
// document (e.g. a settings page with no outer <h1>) sets `value={1}`.
const HeadingLevelContext = createContext(2);

export type HeadingTag = "h1" | "h2" | "h3" | "h4" | "h5" | "h6";

export function useHeadingLevel(): number {
  return Math.min(Math.max(use(HeadingLevelContext), 1), 6);
}

export function useHeadingTag(): HeadingTag {
  return `h${useHeadingLevel()}` as HeadingTag;
}

export function HeadingLevel({
  value,
  children,
}: {
  value?: number;
  children: ReactNode;
}) {
  const current = use(HeadingLevelContext);
  return (
    <HeadingLevelContext value={value ?? current + 1}>
      {children}
    </HeadingLevelContext>
  );
}
