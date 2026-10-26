import { createContext, useContext, type ReactNode } from "react";
import { type ComposerStatePort } from "@/features/composer/application/state";
export type { ComposerStatePort as ComposerAccess } from "@/features/composer/application/state";
const ComposerContext = createContext<ComposerStatePort | null>(null);
export function ComposerProvider({
  value,
  children,
}: {
  value: ComposerStatePort;
  children: ReactNode;
}) {
  return <ComposerContext value={value}>{children}</ComposerContext>;
}
export function useComposer(): ComposerStatePort {
  const value = useContext(ComposerContext);
  if (!value) throw new Error("ComposerProvider is missing");
  return value;
}
