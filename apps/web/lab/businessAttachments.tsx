import { useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { TextArea } from "@flame/ui";
import {
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/public/attachments";
import { formatFileReference } from "@/features/composer/application/draftContext";
import { ComposerAttachments } from "@/features/composer/ui/ComposerAttachments";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { AgentComposerSurface } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import { businessMessages as messages } from "./businessMessages";
import { attachmentVariants, businessVariant } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const LONG_IMAGE_NAME =
  "workspace-review-panel-screenshot-with-annotated-attachment-tray-2026-11-07.png";
const LONG_REFERENCE =
  "apps/web/src/features/composer/ui/composer-attachment-tray-layout-with-a-long-referenced-file-name.tsx";
const KNOWN_PATHS: ReadonlySet<string> = new Set([LONG_REFERENCE]);
const NO_PASTES: readonly PastedText[] = [];

function ignorePaste() {}

export function AttachmentsExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(attachmentVariants, variant);
  const t = useT();
  const input = useRef<HTMLTextAreaElement>(null);
  const [value, setValue] = useState(() => {
    const draft = localize(messages.samples.attachmentDraft, locale);
    return scenario.id === "reference" ? `${draft} ${formatFileReference(LONG_REFERENCE)}` : draft;
  });
  const [images, setImages] = useState<readonly ComposerImageAttachment[]>(() =>
    scenario.id === "reference"
      ? []
      : [{ id: "sample-image", status: scenario.id, name: LONG_IMAGE_NAME }],
  );
  const changeValue = (next: string) => {
    setValue(next);
    observe("composer.draft.length", next.length);
  };
  return (
    <AgentComposerSurface>
      <div {...stylex.props(composerStyles.editorInset)}>
        <ComposerAttachments
          images={images}
          pastes={NO_PASTES}
          value={value}
          knownPaths={KNOWN_PATHS}
          onChange={changeValue}
          onRemoveImage={(id) => {
            setImages((current) => current.filter((image) => image.id !== id));
            observe("composer.attachment.remove", "image");
          }}
          onRemovePaste={ignorePaste}
          onEditPaste={() => false}
          onRestorePaste={ignorePaste}
          composerInput={input}
        />
        <TextArea
          variant="bare"
          data-chrome-focus=""
          size="prose"
          ref={input}
          aria-label={t("composer.input.label")}
          value={value}
          onChange={(event) => changeValue(event.target.value)}
          rows={1}
          autosize
          className={stylex.props(composerStyles.editor).className}
        />
      </div>
    </AgentComposerSurface>
  );
}
