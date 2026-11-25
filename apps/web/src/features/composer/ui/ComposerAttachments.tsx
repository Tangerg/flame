import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type RefObject, useRef, useState } from "react";
import { clsx } from "clsx";
import {
  type ComposerDraftReference,
  type ComposerImage,
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { Chip } from "@/ui/index";
import { LightboxDialog, ButtonPrimitive, ModalDialog } from "@flame/ui";

import { TextEditorDialog } from "@/ui/index";
import { reveal } from "@/ui/index";
import { IconButton } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { face, type as typeStep } from "@flame/ui/tokens.stylex";
interface Props {
  images: readonly ComposerImageAttachment[];
  pastes: readonly PastedText[];
  onRemoveImage: (id: string) => void;
  onRemovePaste: (id: string) => void;
  onEditPaste: (id: string, text: string) => boolean;
  onRestorePaste: (id: string, text: string) => void;
  references: readonly ComposerDraftReference[];
  onRemoveReference: (id: string) => void;
  composerInput: RefObject<HTMLElement | null>;
}

export function ComposerAttachments({
  images,
  pastes,
  onRemoveImage,
  onRemovePaste,
  onEditPaste,
  onRestorePaste,
  references,
  onRemoveReference,
  composerInput,
}: Props) {
  const t = useT();
  const removeImage = (id: string) => {
    onRemoveImage(id);
    composerInput.current?.focus();
  };
  const removePaste = (id: string) => {
    onRemovePaste(id);
    composerInput.current?.focus();
  };
  const removeReference = (id: string) => {
    onRemoveReference(id);
    composerInput.current?.focus();
  };
  const imageStatus = (image: ComposerImageAttachment) =>
    t(
      image.status === "pending"
        ? "composer.image.reading"
        : image.status === "failed"
          ? "composer.image.failed"
          : "composer.image.ready",
      { name: image.name || t("composer.image.preview") },
    );
  return (
    <>
      <span role="status" className="sr-only">
        {images.map(imageStatus).join(". ")}
      </span>
      {(images.length > 0 || pastes.length > 0 || references.length > 0) && (
        <div data-slot="composer-attachments" {...stylex.props(composerStyles.tray)}>
          <AnimatePresence initial={false}>
            {references.map((reference) => (
              <motion.div
                key={reference.id}
                {...chipPresence}
                {...stylex.props(composerStyles.attachment)}
              >
                <ReferenceChip
                  reference={reference}
                  onRemove={() => removeReference(reference.id)}
                />
              </motion.div>
            ))}
            {images.map((image) => (
              <motion.div
                key={image.id}
                {...chipPresence}
                {...stylex.props(composerStyles.attachment)}
              >
                {image.status === "ready" ? (
                  <ImageThumb image={image} onRemove={() => removeImage(image.id)} />
                ) : (
                  <Chip
                    icon="file-text"
                    kind="attached"
                    title={imageStatus(image)}
                    onClose={() => removeImage(image.id)}
                    closeLabel={t("composer.removeImage")}
                  >
                    {imageStatus(image)}
                  </Chip>
                )}
              </motion.div>
            ))}
            {pastes.map((paste) => (
              <motion.div
                key={paste.id}
                {...chipPresence}
                {...stylex.props(composerStyles.attachment)}
              >
                <PasteChip
                  paste={paste}
                  composerInput={composerInput}
                  onRemove={() => removePaste(paste.id)}
                  onEdit={(text) => onEditPaste(paste.id, text)}
                  onRestore={(text) => onRestorePaste(paste.id, text)}
                />
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}
    </>
  );
}

function ImageThumb({ image, onRemove }: { image: ComposerImage; onRemove: () => void }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const src = `data:${image.mime};base64,${image.data}`;
  const name = image.name ?? t("composer.image.preview");
  return (
    <div className={clsx("media-edge", stylex.props(reveal.host, composerStyles.thumb).className)}>
      <LightboxDialog
        open={open}
        onOpenChange={setOpen}
        title={name}
        trigger={
          <ButtonPrimitive
            aria-label={t("composer.image.preview")}
            title={image.name}
            {...stylex.props(composerStyles.thumbOpen)}
          >
            <img src={src} alt={image.name ?? ""} {...stylex.props(composerStyles.thumbImage)} />
          </ButtonPrimitive>
        }
      >
        <img src={src} alt={image.name ?? ""} {...stylex.props(composerStyles.previewImage)} />
      </LightboxDialog>
      <IconButton
        icon={iconGlyph("x")}
        size="xs"
        title={t("composer.removeImage")}
        aria-label={t("composer.removeImage")}
        onClick={onRemove}
        data-reveal="hover"
        className={stylex.props(reveal.shown, composerStyles.thumbRemove).className}
      />
    </div>
  );
}

// Opening a reference shows exactly what the model will receive; only removing
// it changes the draft, because its producer owns what it points at.
function ReferenceChip({
  reference,
  onRemove,
}: {
  reference: ComposerDraftReference;
  onRemove: () => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Chip
        icon="crosshair"
        kind="attached"
        title={t("composer.reference.review")}
        onOpen={() => setOpen(true)}
        onClose={onRemove}
        closeLabel={t("composer.reference.remove")}
      >
        {reference.label}
      </Chip>
      <ModalDialog
        open={open}
        onOpenChange={setOpen}
        title={reference.label}
        closeLabel={t("common.close")}
        bodyStyles={[composerStyles.referenceText, face.mono, typeStep.code]}
      >
        {reference.text}
      </ModalDialog>
    </>
  );
}

interface ReviewExit {
  origin: Element | null;
  returnTo: "chip" | HTMLElement | null;
}

function PasteChip({
  paste,
  composerInput,
  onRemove,
  onEdit,
  onRestore,
}: {
  paste: PastedText;
  composerInput: RefObject<HTMLElement | null>;
  onRemove: () => void;
  onEdit: (text: string) => boolean;
  onRestore: (text: string) => void;
}) {
  const t = useT();
  const [draft, setDraft] = useState<string | null>(null);
  const exit = useRef<ReviewExit | null>(null);
  const finalFocus = () => {
    if (!exit.current?.origin) return false;
    const { origin, returnTo } = exit.current;
    const active = origin.ownerDocument.activeElement;
    if (active !== origin && active !== origin.ownerDocument.body) return false;
    if (returnTo === "chip") return true;
    if (!returnTo?.isConnected) return false;
    return returnTo;
  };
  const label =
    paste.lines > 1
      ? t("composer.paste.lines", { count: paste.lines })
      : t("composer.paste.chars", { count: paste.text.length });
  return (
    <>
      <Chip
        icon="file-text"
        kind="attached"
        title={t("composer.paste.review")}
        onOpen={() => {
          exit.current = null;
          setDraft(paste.text);
        }}
        onClose={onRemove}
        closeLabel={t("composer.paste.remove")}
      >
        {label}
      </Chip>
      <TextEditorDialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (open) return;
          exit.current = { origin: document.activeElement, returnTo: "chip" };
          setDraft(null);
        }}
        title={t("composer.paste.title")}
        closeLabel={t("common.close")}
        finalFocus={finalFocus}
        value={draft ?? ""}
        onChange={setDraft}
        font="mono"
        cancelLabel={t("common.cancel")}
        saveLabel={t("composer.paste.save")}
        savingLabel={t("composer.paste.save")}
        onSave={() => {
          const closing: ReviewExit = {
            origin: document.activeElement,
            returnTo: composerInput.current,
          };
          exit.current = closing;
          if (onEdit(draft ?? "")) closing.returnTo = "chip";
          setDraft(null);
        }}
        secondaryAction={{
          label: t("composer.paste.restore"),
          onClick: () => {
            exit.current = { origin: document.activeElement, returnTo: composerInput.current };
            onRestore(draft ?? "");
            setDraft(null);
          },
        }}
      />
    </>
  );
}
