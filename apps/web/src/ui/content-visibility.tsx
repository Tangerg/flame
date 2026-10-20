import { createContext, use, type ReactNode } from "react";
const ContentVisibleContext = createContext(true);

export function ContentVisibility({
  visible,
  children,
}: {
  visible: boolean;
  children: ReactNode;
}) {
  const inherited = use(ContentVisibleContext);
  return <ContentVisibleContext value={inherited && visible}>{children}</ContentVisibleContext>;
}

export function useContentVisible(): boolean {
  return use(ContentVisibleContext);
}
