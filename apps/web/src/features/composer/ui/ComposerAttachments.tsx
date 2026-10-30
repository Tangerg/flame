import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type RefObject, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import {
  type ComposerImage,
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { basename } from "@/lib/path";
import { Chip } from "@/ui/index";
import { LightboxDialog } from "@flame/ui";
import { Pressable } from "@/ui/index";
import { TextEditorDialog } from "@/ui/index";
import { reveal } from "@/ui/index";
import { IconButton } from "@flame/ui";
import { Icon } from "@/ui/icons/index";
import { fileIconName } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { draftMentions, removeMention } from "@/features/composer/application/draftContext";
import { composerStyles } from "@/features/composer/ui/composerStyles";
interface Props {
  images: readonly ComposerImageAttachment[];
  pastes: readonly PastedText[];
  value: string;
  knownPaths: ReadonlySet<string>;
  onChange: (value: string) => void;
  onRemoveImage: (id: string) => void;
  onRemovePaste: (id: string) => void;
  onEditPaste: (id: string, text: string) => boolean;
  onRestorePaste: (id: string, text: string) => void;
  composerInput: RefObject<HTMLTextAreaElement | null>;
}

export function ComposerAttachments({
  images,
  pastes,
  value,
  knownPaths,
  onChange,
  onRemoveImage,
  onRemovePaste,
  onEditPaste,
  onRestorePaste,
  composerInput,
}: Props) {
  const t = useT();
  const mentions = draftMentions(value, knownPaths);
  const removeImage = (id: string) => {
    onRemoveImage(id);
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
      {(mentions.length > 0 || images.length > 0 || pastes.length > 0) && (
        <div data-slot="composer-attachments" {...stylex.props(composerStyles.tray)}>
          <AnimatePresence initial={false}>
            {images.map((image) => (
              <motion.div key={image.id} {...chipPresence}>
                {image.status === "ready" ? (
                  <ImageThumb image={image} onRemove={() => removeImage(image.id)} />
                ) : (
                  <Chip
                    icon="file-text"
                    kind="attached"
                    title={image.name}
                    onClose={() => removeImage(image.id)}
                    closeLabel={t("composer.removeImage")}
                  >
                    {imageStatus(image)}
                  </Chip>
                )}
              </motion.div>
            ))}
            {mentions.map((mention) => (
              <motion.div key={`${mention.start}:${mention.path}`} {...chipPresence}>
                <Chip
                  icon={fileIconName(mention.path)}
                  title={mention.path}
                  onClose={() => onChange(removeMention(value, mention))}
                >
                  {basename(mention.path)}
                </Chip>
              </motion.div>
            ))}
            {pastes.map((paste) => (
              <motion.div key={paste.id} {...chipPresence}>
                <PasteChip
                  paste={paste}
                  composerInput={composerInput}
                  onRemove={() => onRemovePaste(paste.id)}
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
    <div className={cn("media-edge", stylex.props(reveal.host, composerStyles.thumb).className)}>
      <LightboxDialog
        open={open}
        onOpenChange={setOpen}
        title={name}
        trigger={
          <Pressable
            aria-label={t("composer.image.preview")}
            title={image.name}
            {...stylex.props(composerStyles.thumbOpen)}
          >
            <img src={src} alt={image.name ?? ""} {...stylex.props(composerStyles.thumbImage)} />
          </Pressable>
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

interface ReviewExit {
  origin: Element | null;
  returnTo: "chip" | HTMLTextAreaElement | null;
}

function PasteChip({
  paste,
  composerInput,
  onRemove,
  onEdit,
  onRestore,
}: {
  paste: PastedText;
  composerInput: RefObject<HTMLTextAreaElement | null>;
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
    returnTo.setSelectionRange(returnTo.value.length, returnTo.value.length);
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
        icon={<Icon name="file-text" size="md" />}
        title={t("composer.paste.title")}
        closeLabel={t("common.close")}
        label={t("composer.paste.title")}
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
