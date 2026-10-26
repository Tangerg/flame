import { CodeHighlightProvider, FontScaleProvider } from "@flame/ui";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { useUiFontSize } from "@/lib/appearance";
import { type AnyPlugin } from "dougong";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { queryClient } from "@/lib/queryClient";
import { PluginProvider } from "@/extensions/host/PluginProvider";
import { AppRouter, type AppNavigation } from "@/router";
function App({ plugins, navigation }: { plugins: AnyPlugin[]; navigation: AppNavigation }) {
  const fontSize = useUiFontSize();
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <PluginProvider plugins={plugins}>
          <FontScaleProvider fontSize={fontSize}>
            <CodeHighlightProvider component={HighlightedCodeSource}>
              <AppRouter navigation={navigation} />
            </CodeHighlightProvider>
          </FontScaleProvider>
        </PluginProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}

export default App;
