import {
  createContext,
  use,
  useMemo,
  type ReactNode,
  type RefObject,
} from "react";

export interface ContentVisibilityValue {
  visible: boolean;
  fallbackFocus?: RefObject<HTMLElement | null>;
}

export const ContentVisibilityContext = createContext<ContentVisibilityValue>({
  visible: true,
});

export function ContentVisibility({
  visible,
  fallbackFocus,
  children,
}: {
  visible: boolean;
  fallbackFocus?: RefObject<HTMLElement | null>;
  children: ReactNode;
}) {
  const inherited = use(ContentVisibilityContext);
  const combinedVisible = inherited.visible && visible;
  const combinedFallback = fallbackFocus ?? inherited.fallbackFocus;
  const value = useMemo(
    () => ({ visible: combinedVisible, fallbackFocus: combinedFallback }),
    [combinedVisible, combinedFallback],
  );
  return (
    <ContentVisibilityContext value={value}>
      {children}
    </ContentVisibilityContext>
  );
}

export function useContentVisible(): boolean {
  return use(ContentVisibilityContext).visible;
}
