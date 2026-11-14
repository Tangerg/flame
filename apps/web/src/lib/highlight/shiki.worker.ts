import { renderHighlight } from "./shikiEngine";
import { failureDiagnostics, failureMessage } from "@/lib/diagnostics";
import type { LanguageRegistration } from "shiki/core";

export interface HighlightRequest {
  id: number;
  code: string;
  lang: string;
  theme: string;
  grammars: LanguageRegistration[];
}

export type HighlightResponse =
  | { id: number; status: "ready"; html: string; lang: string; duration: number }
  | { id: number; status: "failed"; error: { message: string; details: string } };

const worker = globalThis as unknown as Pick<Worker, "onmessage" | "postMessage">;
worker.onmessage = async (event: MessageEvent<HighlightRequest>) => {
  const { id, code, lang, theme, grammars } = event.data;
  let response: HighlightResponse;
  try {
    response = { id, status: "ready", ...(await renderHighlight(code, lang, theme, grammars)) };
  } catch (cause) {
    response = {
      id,
      status: "failed",
      error: {
        message: failureMessage(cause),
        details: failureDiagnostics("code highlight", cause),
      },
    };
  }
  worker.postMessage(response);
};
