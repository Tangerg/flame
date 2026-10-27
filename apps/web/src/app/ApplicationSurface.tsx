import { type ReactNode } from "react";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { useUiFontSize } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";

// Both the application root and separately owned native component roots use
// these pure presentation providers. Starting the private UI kernel stays in App.
export function ApplicationSurface({ children }: { children: ReactNode }) {
  const fontSize = useUiFontSize();
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <FontScaleProvider fontSize={fontSize}>
          <CodeHighlightProvider component={HighlightedCodeSource}>
            <TooltipProvider>{children}</TooltipProvider>
          </CodeHighlightProvider>
        </FontScaleProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}
