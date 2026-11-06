import * as stylex from "@stylexjs/stylex";
import { companion } from "@flame/brand-assets";
import {
  Callout,
  CodeBlock,
  DiffTable,
  EvidenceImage,
  ImageViewport,
  type CalloutKind,
  type ComparisonRow,
} from "@flame/ui";
import { foundationMessages } from "./foundationMessages";
import { foundationStyles as styles } from "./foundationStyles";
import { foundationViews as views } from "./foundationVariants";
import type { LabExampleProps } from "./story";

const SHORT_CODE = "const result = await preview.read();\nreturn result;";
const LONG_CODE = [
  'type Preview = Readonly<{ id: string; title: string; state: "ready" | "loading" }>;',
  "",
  "function describe(preview: Preview): string {",
  "  return `${preview.id}: ${preview.title} (${preview.state}) — this intentionally long source line demonstrates the shared reader and its wrap control.`;",
  "}",
].join("\n");

const DIFF_ROWS: readonly ComparisonRow[] = [
  { type: "hunk", text: "@@ -1,4 +1,4 @@" },
  { type: "context", code: "function title(value: string) {", leftLine: 1, rightLine: 1 },
  { type: "deleted", code: '  return "Preview: " + value;', leftLine: 2 },
  { type: "added", code: "  return `Preview: ${value}`;", rightLine: 2 },
  { type: "context", code: "}", leftLine: 3, rightLine: 3 },
  { type: "context", code: 'title("Flame");', leftLine: 4, rightLine: 4, noNewline: true },
];

function calloutKind(value: string): CalloutKind {
  const { note, tip, important, warning, caution } = views;
  if (
    value === note.id ||
    value === tip.id ||
    value === important.id ||
    value === warning.id ||
    value === caution.id
  )
    return value;
  throw new Error(`unknown foundation callout: ${value}`);
}

export function CodeExample({ variant, locale }: LabExampleProps) {
  const copy = foundationMessages[locale];
  return (
    <CodeBlock
      label={copy.exampleCode}
      language="typescript"
      caption="preview.ts"
      labels={{ copy: copy.copyCode, copied: copy.copied, wrap: copy.wrap, unwrap: copy.unwrap }}
    >
      {variant === views.long.id ? LONG_CODE : SHORT_CODE}
    </CodeBlock>
  );
}

export function CalloutExample({ variant, locale }: LabExampleProps) {
  const copy = foundationMessages[locale];
  return (
    <Callout kind={calloutKind(variant)}>
      <p>{copy.calloutBody}</p>
    </Callout>
  );
}

export function DiffExample({ variant, locale }: LabExampleProps) {
  const copy = foundationMessages[locale];
  return (
    <DiffTable
      rows={DIFF_ROWS}
      layout={variant === views.split.id ? "split" : "unified"}
      label={copy.difference}
      labels={{
        region: copy.diffRegion,
        beforeLine: copy.beforeLine,
        beforeCode: copy.beforeCode,
        afterLine: copy.afterLine,
        afterCode: copy.afterCode,
        changedCode: copy.changedCode,
      }}
    />
  );
}

export function ImageExample({ variant, locale }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const labels = {
    zoomIn: copy.zoomIn,
    zoomOut: copy.zoomOut,
    view: copy.viewImage,
    close: copy.close,
    description: copy.imageDescription,
  };
  if (variant === views.viewport.id)
    return (
      <div {...stylex.props(styles.image)}>
        <ImageViewport
          image={{ src: companion, alt: copy.image }}
          label={copy.image}
          labels={labels}
        />
      </div>
    );
  return <EvidenceImage src={companion} alt={copy.image} labels={labels} />;
}
