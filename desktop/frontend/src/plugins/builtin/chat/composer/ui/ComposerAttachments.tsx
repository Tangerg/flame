import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { basename } from "@/lib/path";
import { Chip, Icon, IconButton, Tooltip } from "@/ui";
import { useT } from "@/lib/i18n";
import { draftMentions, removeMention } from "../application/draftContext";

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
        <div className="flex flex-wrap gap-2 pb-1 pt-1">
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
        <div className="flex flex-wrap gap-1.5 pb-1 pt-1">
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
    <div className="flex flex-wrap gap-1.5 pt-1 pb-0.5">
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
    <div className="group relative h-14 w-14 overflow-hidden rounded-[var(--composer-attachment-radius)] media-edge">
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        className="h-full w-full object-cover"
      />
      <IconButton
        icon="x"
        size="xs"
        title={t("composer.removeImage")}
        aria-label={t("composer.removeImage")}
        onClick={onRemove}
        data-reveal="hover"
        className="absolute right-0.5 top-0.5 rounded-full bg-media-scrim text-on-media opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
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
    <Tooltip label={preview}>
      <span className="group inline-flex h-6 max-w-[220px] items-center gap-1.5 rounded-full bg-surface-2 pl-2.5 pr-1.5 font-mono text-ui-sm text-fg-muted">
        <Icon name="filetext" size="xs" className="shrink-0 text-fg-faint" />
        <span className="truncate">{label}</span>
        <IconButton
          icon="x"
          size="xs"
          title={t("composer.paste.remove")}
          aria-label={t("composer.paste.remove")}
          onClick={onRemove}
          className="shrink-0 rounded-full text-fg-faint hover:text-fg"
        />
      </span>
    </Tooltip>
  );
}
