import { useEffect, useMemo } from "react";
import { createTranslator, type ComponentContext } from "@flame/component-sdk";
import { asSessionId } from "@flame/runtime-client";
import { stringifyJSON } from "@qimu/sdk/json";
import { catalogs } from "./catalogs";

type Format = "md" | "json" | "trajectory";

export async function saveSessionExport(
  context: ComponentContext,
  sessionId: string,
  format: Format,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  let content: string;
  if (format === "trajectory") {
    const response = await context.track(() =>
      context.runtime.sessions.exportTrajectory(
        { sessionId: asSessionId(sessionId) },
        signal,
      ),
    );
    content = stringifyJSON(response.trajectory);
  } else {
    const response = await context.track(() =>
      context.runtime.sessions.export(asSessionId(sessionId), format, signal),
    );
    if (format === "md" && response.markdown !== undefined)
      content = response.markdown;
    else if (format === "json" && response.artifact !== undefined)
      content = stringifyJSON(response.artifact);
    else
      throw new Error(`Runtime sessions.export omitted its ${format} result.`);
  }
  signal.throwIfAborted();
  const stamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
  const suffix = format === "trajectory" ? "-trajectory" : "";
  await context.saveFile({
    filename: `flame-${sessionId}${suffix}-${stamp}.${format === "md" ? "md" : "json"}`,
    mime:
      format === "md"
        ? "text/markdown;charset=utf-8"
        : "application/json;charset=utf-8",
    bytes: new TextEncoder().encode(content),
  });
}

export function SessionExports({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(catalogs, context.locale),
    [context.locale],
  );
  useEffect(() => {
    const commands = [
      { id: "markdown", label: t("export.markdown"), format: "md" },
      { id: "json", label: t("export.json"), format: "json" },
      { id: "trajectory", label: t("export.trajectory"), format: "trajectory" },
    ] as const;
    const stops = commands.map(({ id, label, format }) =>
      context.registerCommand({
        id,
        label,
        run: ({ sessionId, signal }) =>
          sessionId
            ? saveSessionExport(context, sessionId, format, signal)
            : undefined,
      }),
    );
    return () => {
      for (const stop of stops) stop();
    };
  }, [context, t]);
  return null;
}
