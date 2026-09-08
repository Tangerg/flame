import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { basename } from "@/lib/path";
import { Chip, IconButton, gap, reveal } from "@/ui";
import { useT } from "@/lib/i18n";
import { draftMentions, removeMention } from "../application/draftContext";
import { composerStyles } from "./composerStyles";

interface Props {
  images: readonly ComposerImage[];
  pastes: readonly PastedText[];
  value: string;
  onChange: (value: string) => void;
  onRemoveImage: (id: string) => void;
  onRemovePaste: (id: string) => void;
}

export function ComposerAttachments({
  images,
  pastes,
  value,
  onChange,
  onRemoveImage,
  onRemovePaste,
}: Props) {
  return (
    <>
      <DraftContext value={value} onChange={onChange} />
      {images.length > 0 && (
        <div {...stylex.props(composerStyles.attachmentRow, gap.s2)}>
          <AnimatePresence initial={false}>
            {images.map((img) => (
              <motion.div key={img.id} {...chipPresence}>
                <ImageThumb image={img} onRemove={() => onRemoveImage(img.id)} />
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}
      {pastes.length > 0 && (
        <div {...stylex.props(composerStyles.attachmentRow, gap.s1_5)}>
          <AnimatePresence initial={false}>
            {pastes.map((p) => (
              <motion.div key={p.id} {...chipPresence}>
                <PasteChip paste={p} onRemove={() => onRemovePaste(p.id)} />
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}
    </>
  );
}

function DraftContext({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const mentions = draftMentions(value);
  if (mentions.length === 0) return null;
  return (
    <div {...stylex.props(composerStyles.attachmentRow, gap.s1_5)}>
      <AnimatePresence initial={false}>
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
      </AnimatePresence>
    </div>
  );
}

function ImageThumb({ image, onRemove }: { image: ComposerImage; onRemove: () => void }) {
  const t = useT();
  return (
    <div
      // `media-edge` is the mechanism `globals.css` owns: the hairline an image wears so its
      // own light edge does not read as the surface behind it.
      className={cn("media-edge", stylex.props(reveal.host, composerStyles.thumb).className)}
    >
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        {...stylex.props(composerStyles.thumbImage)}
      />
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

const PREVIEW_LIMIT = 160;

// Cutting by code UNIT lands inside any non-BMP character, and half of one renders as a
// replacement glyph before the ellipsis.
function previewOf(text: string): string {
  if (text.length <= PREVIEW_LIMIT) return text;
  const last = text.charCodeAt(PREVIEW_LIMIT - 1);
  const cut = last >= 0xd800 && last <= 0xdbff ? PREVIEW_LIMIT - 1 : PREVIEW_LIMIT;
  return `${text.slice(0, cut)}…`;
}

function PasteChip({ paste, onRemove }: { paste: PastedText; onRemove: () => void }) {
  const t = useT();
  const preview = previewOf(paste.text);
  const label =
    paste.lines > 1
      ? t("composer.paste.lines", { count: paste.lines })
      : t("composer.paste.chars", { count: paste.text.length });
  return (
    <Chip
      icon="filetext"
      kind="attached"
      title={preview}
      onClose={onRemove}
      closeLabel={t("composer.paste.remove")}
    >
      {label}
    </Chip>
  );
}
