import * as stylex from "@stylexjs/stylex";
import { useMemo, useState } from "react";
import { X } from "@flame/icons/react";
import { ButtonPrimitive } from "./button-primitive";
import { IconButton } from "./icon-button";
import {
  ImageViewport,
  type ImageSource,
  type ImageViewportLabels,
} from "./image-viewport";
import { LightboxDialog } from "./lightbox-dialog";
import { ScrollRegion } from "./scroll-region";
import { color, space, type } from "./tokens.stylex";

export interface EvidenceImageLabels extends ImageViewportLabels {
  view: string;
  close: string;
  description: string;
}

const styles = stylex.create({
  trigger: {
    display: "inline-flex",
    minWidth: "var(--control-target-min)",
    minHeight: "var(--control-target-min)",
    maxWidth: "100%",
    alignItems: "center",
    justifyContent: "center",
    padding: 0,
    borderWidth: 0,
    borderRadius: 0,
    backgroundColor: "transparent",
    verticalAlign: "middle",
    cursor: "zoom-in",
  },
  image: {
    display: "block",
    width: "auto",
    height: "auto",
    maxWidth: "100%",
    maxHeight: "20rem",
    objectFit: "contain",
    outline: "1px solid var(--color-media-edge)",
    outlineOffset: "-1px",
  },
  frame: {
    display: "flex",
    width: "100%",
    height: "100%",
    minWidth: 0,
    minHeight: 0,
    flexDirection: "column",
  },
  header: {
    display: "flex",
    alignItems: "flex-start",
    flexShrink: 0,
    minWidth: 0,
    gap: space.s2,
    paddingBlockStart: space.s3,
    paddingInlineStart: `max(${space.s3}, var(--window-controls-gutter, 0px))`,
    paddingInlineEnd: space.s3,
    color: color.onMedia,
  },
  identity: {
    flex: 1,
    minWidth: 0,
    maxHeight: "25dvh",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    paddingBlock: space.s2,
    userSelect: "text",
    overscrollBehavior: "contain",
  },
});

type EvidenceImageProps = Pick<
  ImageSource,
  "src" | "alt" | "width" | "height"
> & {
  labels: EvidenceImageLabels;
  loading?: "eager" | "lazy";
  decoding?: "async" | "sync" | "auto";
};

export function EvidenceImage(props: EvidenceImageProps) {
  return <CapturedImage key={props.src} {...props} />;
}

function CapturedImage({
  src,
  alt,
  labels,
  width,
  height,
  loading,
  decoding,
}: EvidenceImageProps) {
  const [open, setOpen] = useState(false);
  const image = useMemo(
    () => ({ src, alt, width, height }),
    [src, alt, width, height],
  );
  const label = alt || labels.view;
  return (
    <LightboxDialog
      open={open}
      onOpenChange={setOpen}
      kind="media"
      title={label}
      trigger={
        <ButtonPrimitive
          aria-label={alt ? `${labels.view}: ${alt}` : labels.view}
          data-focus-outset=""
          {...stylex.props(styles.trigger)}
        >
          <img
            src={src}
            alt={alt}
            width={width}
            height={height}
            loading={loading}
            decoding={decoding}
            {...stylex.props(styles.image)}
          />
        </ButtonPrimitive>
      }
    >
      <div {...stylex.props(styles.frame)}>
        <header {...stylex.props(styles.header)}>
          <ScrollRegion
            as="span"
            label={labels.description}
            styles={[styles.identity, type.uiSm]}
          >
            {label}
          </ScrollRegion>
          <IconButton
            icon={X}
            aria-label={labels.close}
            onClick={() => setOpen(false)}
            variant="media"
            size="xl"
          />
        </header>
        <ImageViewport image={image} label={label} labels={labels} />
      </div>
    </LightboxDialog>
  );
}
