import { lazy, Suspense, useEffect } from "react";
import { useT } from "@/lib/i18n/index";
import { SkeletonText } from "@/ui/index";
import { type MarkdownMessageProps } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
import { useVisibleTextMaterial } from "@/features/chat/message/ui/messageVisibleMaterial";
const loadRenderer = () => import("@/features/chat/message/ui/markdown/MarkdownRenderer");

const MarkdownRenderer = lazy(() => loadRenderer().then((m) => ({ default: m.MarkdownRenderer })));

function PendingMarkdown() {
  const t = useT();
  useVisibleTextMaterial(false);
  return (
    <div data-surface-pending="">
      <SkeletonText label={t("common.loading")} />
    </div>
  );
}

export function MarkdownMessage(props: MarkdownMessageProps) {
  return (
    <Suspense fallback={<PendingMarkdown />}>
      <MarkdownRenderer {...props} />
    </Suspense>
  );
}

export function useWarmMarkdownRenderer(): void {
  useEffect(() => {
    void loadRenderer();
  }, []);
}
