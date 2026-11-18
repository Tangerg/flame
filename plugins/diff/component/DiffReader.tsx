import { CodeCaption, CodeFrame, DiffBody } from "@flame/ui";
import type { Translate } from "@flame/component-sdk";

export function DiffReader({
  path,
  source,
  patch,
  layout,
  t,
}: {
  path: string;
  source: string;
  patch: string;
  layout: "unified" | "split";
  t: Translate;
}) {
  return (
    <CodeFrame data-slot="workspace-diff" data-focus-unit="">
      <CodeCaption
        language="diff"
        code={patch}
        labels={{
          copy: t("patch.copy"),
          wrap: t("patch.wrap"),
          unwrap: t("patch.unwrap"),
        }}
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
