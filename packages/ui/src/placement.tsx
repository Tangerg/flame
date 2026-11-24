import { use, useMemo, type ReactNode } from "react";
import {
  ContentVisibility,
  ContentVisibilityContext,
  type ContentVisibilityValue,
} from "./content-visibility";
import { HeadingLevel, useHeadingLevel } from "./heading-level";
import { PanelHost, usePanelHostDensity, type PanelDensity } from "./panel";

// Facts that belong to where a subtree is shown rather than to the app. A
// separately rooted subtree, such as an installed component, sees none of its
// mount site's providers, so the host reads them there and re-provides them.
export interface Placement {
  density: PanelDensity | null;
  visibility: ContentVisibilityValue;
  headingLevel: number;
}

export function usePlacement(): Placement {
  const density = usePanelHostDensity();
  const visibility = use(ContentVisibilityContext);
  const headingLevel = useHeadingLevel();
  return useMemo(
    () => ({ density, visibility, headingLevel }),
    [density, visibility, headingLevel],
  );
}

export function Placed({
  placement,
  children,
}: {
  placement: Placement;
  children: ReactNode;
}) {
  const content = (
    <ContentVisibility
      visible={placement.visibility.visible}
      fallbackFocus={placement.visibility.fallbackFocus}
    >
      <HeadingLevel value={placement.headingLevel}>{children}</HeadingLevel>
    </ContentVisibility>
  );
  return placement.density ? (
    <PanelHost density={placement.density}>{content}</PanelHost>
  ) : (
    content
  );
}
