import { type ReactNode } from "react";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { useUiFontSize } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";
import { ApplicationCalloutLabels } from "./calloutLabels";

// Both the application root and separately owned native component roots use
// these pure presentation providers. Starting the private UI kernel stays in App.
export function ApplicationSurface({ children }: { children: ReactNode }) {
  const fontSize = useUiFontSize();
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <FontScaleProvider fontSize={fontSize}>
          <CodeHighlightProvider component={HighlightedCodeSource}>
            <ApplicationCalloutLabels>
              <TooltipProvider>{children}</TooltipProvider>
            </ApplicationCalloutLabels>
          </CodeHighlightProvider>
        </FontScaleProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}
