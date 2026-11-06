import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { FormField, TextField } from "@flame/ui";
import { FilePath } from "@/ui/atoms/file-path";
import { Tag } from "@/ui/atoms/tag";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import { businessVariant, fileVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

export function FileIdentityExample(props: LabExampleProps) {
  const scenario = businessVariant(fileVariants, props.variant);
  return <FileIdentityEditor key={scenario.id} {...props} initialPath={scenario.path} />;
}

function FileIdentityEditor({
  locale,
  observe,
  initialPath,
}: LabExampleProps & { initialPath: string }) {
  const [path, setPath] = useState(initialPath);
  return (
    <div {...stylex.props(styles.stack)}>
      <FormField label={localize(messages.samples.path, locale)}>
        <TextField
          value={path}
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          onValueChange={(value) => {
            setPath(value);
            observe("file.path.length", value.length);
          }}
        />
      </FormField>
      <div
        role="group"
        aria-label={localize(messages.samples.pathPreview, locale)}
        {...stylex.props(styles.rows)}
      >
        <FilePath path={path} />
        <Tag>{localize(messages.samples.readOnly, locale)}</Tag>
      </div>
    </div>
  );
}
