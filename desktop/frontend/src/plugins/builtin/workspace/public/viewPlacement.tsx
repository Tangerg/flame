// View placement controls — provided by ChatPanel (the layout owner) and
// consumed by a workspace view's own ViewHeader, so the view gets an
// "open in the dock / close" affordance without ChatPanel reaching into the view
// body. null when not inside a view at all (e.g. the chat itself).

import { createContext, use } from "react";

export interface ViewPlacement {
  /** "full" = the view has the whole content card; "dock" = beside the chat. */
  placement: "full" | "dock";
  splittable: boolean;
  onOpenInDock: () => void;
  /** Dismiss this view — full → back to the chat; dock → close the dock. */
  onClose: () => void;
}

const Ctx = createContext<ViewPlacement | null>(null);

export const ViewPlacementProvider = Ctx.Provider;

export function useViewPlacement(): ViewPlacement | null {
  return use(Ctx);
}
