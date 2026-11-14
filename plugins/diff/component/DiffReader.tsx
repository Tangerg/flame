import { useCallback, useLayoutEffect, useMemo, useRef } from "react";
import {
  i18n,
  registerCustomCSSVariableTheme,
  type FileDiffMetadata,
  type FileDiff as DiffInstance,
} from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import {
  CodeCaption,
  CodeFrame,
  ScrollRegion,
  selectAllInReader,
  scrollWithHorizontalArrow,
  useContentVisible,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import { copyDiffSelection } from "./diffSelection";

registerCustomCSSVariableTheme("flame-workspace-diff", {
  foreground: "var(--color-text)",
  background: "var(--color-sunken)",
  "token-comment": "var(--color-text-muted)",
  "token-keyword": "var(--color-accent-text)",
  "token-string-expression": "var(--color-success)",
  "token-string": "var(--color-success)",
  "token-constant": "var(--color-accent-text)",
  "token-parameter": "var(--color-text-soft)",
  "token-function": "var(--color-info)",
  "token-punctuation": "var(--color-text)",
  "token-link": "var(--color-accent-text)",
  "token-inserted": "var(--color-success)",
  "token-deleted": "var(--color-negative)",
});

const appearance = `:host {
  color-scheme: inherit;
  --diffs-font-family: var(--font-mono);
  --diffs-header-font-family: var(--font-sans);
  --diffs-font-size: var(--fs-code);
  --diffs-line-height: var(--leading-relaxed);
  --diffs-addition-base: var(--color-success);
  --diffs-deletion-base: var(--color-negative);
  --diffs-modified-base: var(--color-info);
}`;

export function DiffReader({
  file,
  patch,
  layout,
  locale,
  t,
}: {
  file: FileDiffMetadata;
  patch: string;
  layout: "unified" | "split";
  locale: string;
  t: Translate;
}) {
  const root = useRef<{
    node: ShadowRoot;
    file: FileDiffMetadata;
    renderer: DiffInstance;
  } | null>(null);
  const visible = useContentVisible();
  useLayoutEffect(() => {
    i18n.set({
      locale,
      messages: {
        unmodifiedLines: t("reader.unmodifiedLines"),
        unknownUnmodifiedLines: t("reader.unknownUnmodifiedLines"),
        expandAll: t("reader.expandAll"),
        noNewlineAtEndOfFile: t("reader.noNewline"),
      },
    });
  }, [locale, t]);
  const rendered = useCallback(
    (
      node: HTMLElement,
      renderer: DiffInstance,
      phase: "mount" | "update" | "unmount",
    ) => {
      if (phase !== "unmount" && node.shadowRoot)
        root.current = { node: node.shadowRoot, file, renderer };
      else if (root.current?.node === node.shadowRoot) root.current = null;
    },
    [file],
  );
  const options = useMemo(
    () => ({
      diffStyle: layout,
      disableFileHeader: true,
      stickyHeader: false,
      theme: "flame-workspace-diff",
      preferredHighlighter: "shiki-js" as const,
      hunkSeparators: "metadata" as const,
      parseDiffOptions: { disableLineMatching: true },
      unsafeCSS: appearance,
      onPostRender: rendered,
    }),
    [layout, rendered],
  );
  return (
    <CodeFrame data-slot="workspace-diff" data-focus-unit="">
      <CodeCaption
        language="diff"
        code={patch}
        wrappable={false}
        labels={{
          copy: t("patch.copy"),
          copied: t("patch.copied"),
          wrap: t("patch.wrap"),
          unwrap: t("patch.unwrap"),
        }}
      >
        {t("patch.accepted")}
      </CodeCaption>
      <ScrollRegion
        label={t("reader.region", { path: file.name })}
        onKeyDown={(event) => {
          if (visible && root.current) {
            selectAllInReader(event, root.current.node);
            const { renderer } = root.current;
            scrollWithHorizontalArrow(event, (delta) => {
              renderer.setCodeScrollLeft(renderer.getCodeScrollLeft() + delta);
            });
          }
        }}
        onCopy={(event) => {
          if (visible && root.current)
            copyDiffSelection(event, root.current.node, root.current.file);
        }}
      >
        <FileDiff fileDiff={file} options={options} disableWorkerPool />
      </ScrollRegion>
    </CodeFrame>
  );
}
