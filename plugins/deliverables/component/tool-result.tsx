import { useMemo } from "react";
import { type ComponentContext, useTranslator } from "@flame/component-sdk";
import { Button, Metadata, Toolbar } from "@flame/ui";
import { z } from "zod/v4";
import { catalogs } from "./catalogs";
import { DELIVERABLES_VIEW, encodeTarget } from "./targets";

// The model's text result for a command that produced a revision: the
// revision summary the backend's model projection writes.
const id = z.string().regex(/^[a-f0-9]{64}$/u);
const producedSchema = z.object({
  value: z.object({
    id,
    documentId: id,
    operation: z.enum([
      "create",
      "import",
      "copy",
      "edit",
      "recalculate",
      "restore",
    ]),
    format: z.enum(["docx", "xlsx", "pptx", "pdf"]),
  }),
});

// A result this card cannot read is left to the host's default rendering.
export function DeliverableResult({ context }: { context: ComponentContext }) {
  const t = useTranslator(catalogs, context.locale);
  const result = context.toolResult?.result;
  const produced = useMemo(() => {
    if (context.toolResult?.status !== "ok" || typeof result !== "string")
      throw new Error("deliverables: a settled successful result is required");
    return producedSchema.parse(JSON.parse(result)).value;
  }, [context.toolResult?.status, result]);
  const views = context.views;
  return (
    <Toolbar label={t("link.result")}>
      <Metadata>
        {t(`operation.${produced.operation}`)} ·{" "}
        {t(`format.${produced.format}`)} · {produced.id.slice(0, 12)}
      </Metadata>
      {views && (
        <Button
          onClick={(event) =>
            views.open(
              DELIVERABLES_VIEW,
              encodeTarget({
                document: produced.documentId,
                revision: produced.id,
              }),
              event.currentTarget,
            )
          }
        >
          {t("link.open")}
        </Button>
      )}
    </Toolbar>
  );
}
