import { createContext, useContext, type ReactNode } from "react";
import { type InstalledPackageState } from "@/features/plugins/adapters/packages";
const InstalledPluginsContext = createContext<InstalledPackageState | null>(null);
export function InstalledPluginsProvider({
  value,
  children,
}: {
  value: InstalledPackageState;
  children: ReactNode;
}) {
  return <InstalledPluginsContext value={value}>{children}</InstalledPluginsContext>;
}
export function useInstalledPlugins(): InstalledPackageState {
  const value = useContext(InstalledPluginsContext);
  if (!value) throw new Error("InstalledPluginsProvider is missing");
  return value;
}
