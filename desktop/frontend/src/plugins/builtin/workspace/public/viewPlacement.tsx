import { createContext, use } from "react";

export interface ViewPlacement {
  placement: "full" | "dock";
  splittable: boolean;
  onOpenInDock: () => void;
  onClose: () => void;
}

const Ctx = createContext<ViewPlacement | null>(null);

export const ViewPlacementProvider = Ctx.Provider;

export function useViewPlacement(): ViewPlacement | null {
  return use(Ctx);
}
