import { type ReactNode } from "react";
import {
  ClipboardReceiptProvider,
  type CopyReceipt,
  FontScaleProvider,
  TooltipProvider,
} from "@flame/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { CodeHighlighting } from "@/lib/highlight/CodeHighlighting";
import { useUiFontSize } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";
import { ApplicationCalloutLabels } from "./calloutLabels";

// Both the application root and separately owned native component roots use
// these pure presentation providers. Starting the private UI kernel stays in App.
export function ApplicationSurface({
  children,
  onCopyReceipt,
}: {
  children: ReactNode;
  onCopyReceipt?(receipt: CopyReceipt): void;
}) {
  const fontSize = useUiFontSize();
  const content = onCopyReceipt ? (
    <ClipboardReceiptProvider onReceipt={onCopyReceipt}>{children}</ClipboardReceiptProvider>
  ) : (
    children
  );
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <FontScaleProvider fontSize={fontSize}>
          <CodeHighlighting>
            <ApplicationCalloutLabels>
              <TooltipProvider>{content}</TooltipProvider>
            </ApplicationCalloutLabels>
          </CodeHighlighting>
        </FontScaleProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}
