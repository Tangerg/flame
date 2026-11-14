import { createHighlightQueue } from "./highlightQueue";
import { loadHighlightLanguage } from "./shikiLanguages";

const queue = createHighlightQueue(async () => {
  const { createShikiWorker } = await import(/* @flame-lazy:syntax-highlighting */ "./shikiWorker");
  return createShikiWorker();
}, loadHighlightLanguage);
export const requestHighlight = queue.request;

const reported = new Set<string>();
export function reportHighlightFailure(cause: string, error: unknown): void {
  if (reported.has(cause)) return;
  reported.add(cause);
  console.error(`[highlight] ${cause}:`, error);
}

export async function disposeHighlighter(): Promise<void> {
  queue.dispose();
}
