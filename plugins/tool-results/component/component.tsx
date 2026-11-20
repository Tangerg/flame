import { useMemo } from "react";
import { type ComponentContext, useTranslator } from "@flame/component-sdk";
import { Metadata, RecordMeta } from "@flame/ui";
import { RecordedOutput } from "./recorded-output";
import { RecordedRows } from "./recorded-rows";
import { resultContent, text } from "./result-content";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

const CHANGE_VERB = {
  added: "change.added",
  modified: "change.modified",
  moved: "change.moved",
  deleted: "change.deleted",
} as const;

export function RecordedResult({ context }: { context: ComponentContext }) {
  const t = useTranslator(CATALOGS, context.locale);
  const tool = context.toolResult;
  const content = useMemo(() => {
    if (!tool || tool.status === "running" || typeof tool.result !== "string")
      throw new Error("A settled captured Tool result is required");
    return resultContent(context.component.componentId, tool, t);
  }, [context.component.componentId, tool, t]);
  if (content.kind === "changes")
    return !content.changes.length ? (
      <Metadata>{t("changes.empty")}</Metadata>
    ) : (
      <table
        aria-label={t("changes.applied")}
        style={{ width: "100%", tableLayout: "auto" }}
      >
        <tbody>
          {content.changes.map((change, index) => {
            const path = text(change.path);
            const status = text(change.status);
            const verb = CHANGE_VERB[status as keyof typeof CHANGE_VERB];
            return (
              <tr key={`${path}:${index}`}>
                <td
                  style={{
                    width: verb ? "1%" : "5em",
                    whiteSpace: verb ? "nowrap" : undefined,
                    overflowWrap: "anywhere",
                    paddingInlineEnd: "var(--layout-content-gap)",
                    verticalAlign: "baseline",
                    color: "var(--color-text-muted)",
                    fontSize: "var(--fs-ui-sm)",
                  }}
                >
                  {verb ? t(verb) : status}
                </td>
                <td
                  style={{
                    verticalAlign: "baseline",
                    overflowWrap: "anywhere",
                    fontFamily: "var(--font-mono)",
                    fontSize: "var(--fs-code)",
                  }}
                >
                  <span title={path}>{path}</span>
                  {typeof change.from === "string" && (
                    <Metadata>
                      {t("changes.from", { from: change.from })}
                    </Metadata>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    );
  if (content.kind === "output")
    return (
      <>
        {tool?.outputPage && (
          <RecordMeta>
            {t("output.byteRange", {
              start: tool.outputPage.offset,
              end: tool.outputPage.nextOffset,
            })}
            {tool.outputPage.shellId &&
              ` · ${t("output.shell", { id: tool.outputPage.shellId })}`}
            {tool.outputPage.status && ` · ${tool.outputPage.status}`}
            {tool.outputPage.hasMore && ` · ${t("result.morePages")}`}
            {tool.outputPage.dropped && ` · ${t("output.incomplete")}`}
          </RecordMeta>
        )}
        {content.text === "" ? (
          <Metadata>{t("output.empty")}</Metadata>
        ) : (
          <RecordedOutput text={content.text} t={t} />
        )}
      </>
    );
  return (
    <RecordedRows
      label={content.label}
      source={tool!.result!}
      recorded={content.rows}
      continues={content.continues}
      notice={content.notice}
      emptyMessage={content.emptyMessage}
      t={t}
    />
  );
}
