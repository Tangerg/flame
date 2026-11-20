import {
  CodeCaption,
  CodeFrame,
  DiffBody,
  type CodeActionLabels,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";

export function DiffReader({
  path,
  source,
  patch,
  layout,
  labels,
  t,
}: {
  path: string;
  source: string;
  patch: string;
  layout: "unified" | "split";
  labels: CodeActionLabels;
  t: Translate;
}) {
  return (
    <CodeFrame data-slot="workspace-diff" data-focus-unit="">
      <CodeCaption
        language="diff"
        code={patch}
        labels={labels}
        wrappable={false}
      >
        {t("patch.accepted")}
      </CodeCaption>
      <DiffBody
        patch={source}
        layout={layout}
        label={t("reader.region", { path })}
      />
    </CodeFrame>
  );
}
