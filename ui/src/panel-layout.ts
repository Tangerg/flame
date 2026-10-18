import { createContext, type RefObject } from "react";

export const PanelLayoutContext = createContext<{
  controlsRef: RefObject<HTMLDivElement | null> | null;
  keepBodyFocusVisible: (target: Element | null) => void;
} | null>(null);
