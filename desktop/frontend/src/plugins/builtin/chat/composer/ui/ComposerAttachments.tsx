import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { basename } from "@/lib/path";
import { Chip, IconButton, LightboxDialog, Pressable, TextEditorDialog, reveal } from "@/ui";
import { Icon } from "@/ui/icons";
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
  onEditPaste: (id: string, text: string) => void;
  onRestorePaste: (id: string) => void;
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
              icon="filetext"
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
              onRemove={() => onRemovePaste(paste.id)}
              onEdit={(text) => onEditPaste(paste.id, text)}
              onRestore={() => onRestorePaste(paste.id)}
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
        icon="x"
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

function PasteChip({
  paste,
  onRemove,
  onEdit,
  onRestore,
}: {
  paste: PastedText;
  onRemove: () => void;
  onEdit: (text: string) => void;
  onRestore: () => void;
}) {
  const t = useT();
  const [draft, setDraft] = useState<string | null>(null);
  const label =
    paste.lines > 1
      ? t("composer.paste.lines", { count: paste.lines })
      : t("composer.paste.chars", { count: paste.text.length });
  return (
    <>
      <Chip
        icon="filetext"
        kind="attached"
        title={t("composer.paste.review")}
        onOpen={() => setDraft(paste.text)}
        onClose={onRemove}
        closeLabel={t("composer.paste.remove")}
      >
        {label}
      </Chip>
      <TextEditorDialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (!open) setDraft(null);
        }}
        icon={<Icon name="filetext" size="md" />}
        title={t("composer.paste.title")}
        closeLabel={t("common.close")}
        label={t("composer.paste.title")}
        value={draft ?? ""}
        onChange={setDraft}
        font="mono"
        cancelLabel={t("common.cancel")}
        saveLabel={t("composer.paste.save")}
        savingLabel={t("composer.paste.save")}
        onSave={() => {
          onEdit(draft ?? "");
          setDraft(null);
        }}
        secondaryAction={{
          label: t("composer.paste.restore"),
          onClick: () => {
            onRestore();
            setDraft(null);
          },
        }}
      />
    </>
  );
}
