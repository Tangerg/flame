import { createContext, useContext, type ReactNode } from "react";
import { AppearanceProvider } from "@/lib/appearance";
import { type ThemeAccess } from "@/features/theme/application/themeAccess";
export type { ThemeAccess } from "@/features/theme/application/themeAccess";
const ThemeContext = createContext<ThemeAccess | null>(null);
export function ThemeProvider({ value, children }: { value: ThemeAccess; children: ReactNode }) {
  return (
    <ThemeContext value={value}>
      <AppearanceProvider value={value.appearance}>{children}</AppearanceProvider>
    </ThemeContext>
  );
}
export function useTheme(): ThemeAccess {
  const value = useContext(ThemeContext);
  if (!value) throw new Error("ThemeProvider is missing");
  return value;
}
