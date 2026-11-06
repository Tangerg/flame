import { type ReactNode } from "react";
import { MotionConfig } from "motion/react";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { useUiFontSize } from "@/lib/appearance";
import { createTheme } from "@/features/theme/adapters/theme";
import { createAppearancePlugins } from "@/features/theme/index";
import { ThemeProvider } from "@/features/theme/public/theme";
import { defaultAppearance } from "@/features/theme/kit/appearance";
import { ApplicationCalloutLabels } from "@/app/calloutLabels";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { i18nPlugin } from "@/features/i18n/index";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { selectLocale } from "@/features/i18n/application/localeSelection";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { visualLocalePackages } from "./scenarios/localePackages";
import type { LabLocale, LabMotion, LabTheme } from "./story";

export interface LabAppearance {
  readonly theme: LabTheme;
  readonly locale: LabLocale;
  readonly motion: LabMotion;
  readonly fontSize: number;
}

export async function createLabEnvironment(initial: LabAppearance) {
  const theme = createTheme(document, window);
  theme.store.setState({
    ...defaultAppearance(),
    theme: initial.theme,
    fontSize: initial.fontSize,
    motionScale: initial.motion === "off" ? 0 : 1,
  });
  await loadPluginsForTest(i18nPlugin, visualLocalePackages, ...createAppearancePlugins(theme));
  let disposed = false;

  async function update(next: LabAppearance) {
    if (disposed) return;
    const locale = lookupExtensionByKey(LOCALE, next.locale === "zh-CN" ? "zh" : "en");
    if (!locale) throw new Error(`Lab locale is unavailable: ${next.locale}`);
    theme.store.setState({
      theme: next.theme,
      motionScale: next.motion === "off" ? 0 : 1,
      fontSize: next.fontSize,
    });
    await selectLocale(locale);
  }

  await update(initial);

  return {
    update,
    Wrapper({ children }: { children: ReactNode }) {
      return (
        <ThemeProvider value={theme.access}>
          <LabPresentation>{children}</LabPresentation>
        </ThemeProvider>
      );
    },
    async dispose() {
      if (disposed) return;
      disposed = true;
      theme.dispose();
      await resetKernelForTest();
    },
  };
}

export type LabEnvironment = Awaited<ReturnType<typeof createLabEnvironment>>;

function LabPresentation({ children }: { children: ReactNode }) {
  const fontSize = useUiFontSize();
  return (
    <MotionConfig reducedMotion="user">
      <FontScaleProvider fontSize={fontSize}>
        <CodeHighlightProvider component={HighlightedCodeSource}>
          <TooltipProvider>
            <ApplicationCalloutLabels>{children}</ApplicationCalloutLabels>
          </TooltipProvider>
        </CodeHighlightProvider>
      </FontScaleProvider>
    </MotionConfig>
  );
}
