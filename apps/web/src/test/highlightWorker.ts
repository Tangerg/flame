import { renderHighlight } from "@/lib/highlight/shikiEngine";
import type { HighlightRequest, HighlightResponse } from "@/lib/highlight/shiki.worker";

// Happy DOM has no Worker execution. Render tests substitute only that platform
// transport; the browser Lab exercises the real Worker and its queue.
export function createShikiWorker(): Worker {
  let retired = false;
  const worker = {
    onmessage: null as ((event: MessageEvent<HighlightResponse>) => void) | null,
    onerror: null as ((event: ErrorEvent) => void) | null,
    terminate() {
      retired = true;
    },
    postMessage(request: HighlightRequest) {
      void renderHighlight(request.code, request.lang, request.theme, request.grammars).then(
        (result) => {
          if (!retired)
            worker.onmessage?.(
              new MessageEvent("message", {
                data: { id: request.id, status: "ready", ...result } satisfies HighlightResponse,
              }),
            );
        },
        (error: unknown) => {
          if (!retired) worker.onerror?.(new ErrorEvent("error", { error }));
        },
      );
    },
  };
  return worker as unknown as Worker;
}
