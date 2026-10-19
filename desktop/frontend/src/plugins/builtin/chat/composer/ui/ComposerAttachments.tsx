import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { type RefObject, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { basename } from "@/lib/path";
import { Chip, LightboxDialog, Pressable, TextEditorDialog, reveal } from "@/ui";
import { IconButton } from "@flame/ui";
import { Icon, fileIconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { draftMentions, removeMention } from "../application/draftContext";
import { composerStyles } from "./composerStyles";

interface Props {
  images: readonly ComposerImage[];
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
  const mentions = draftMentions(value, knownPaths);
  if (mentions.length === 0 && images.length === 0 && pastes.length === 0) return null;
  return (
    <div data-slot="composer-attachments" {...stylex.props(composerStyles.tray)}>
      <AnimatePresence initial={false}>
        {images.map((image) => (
          <motion.div key={image.id} {...chipPresence}>
            <ImageThumb image={image} onRemove={() => onRemoveImage(image.id)} />
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
