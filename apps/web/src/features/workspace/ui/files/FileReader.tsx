import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import {
  useCallback,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEventHandler,
  type FormEvent,
  type KeyboardEventHandler,
  type MouseEvent,
  type Ref,
  type RefObject,
} from "react";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { Popover } from "@/ui/index";
import { TextButton } from "@/ui/index";
import {
  Button,
  copyReaderSelection,
  IconButton,
  isImeKey,
  scrollReaderWithForeignSelection,
  scrollWithHorizontalArrow,
  selectAllInReader,
  TextField,
  vocab,
} from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { FileView } from "@/features/workspace/ui/files/FileView";
import { type WorkspaceFileContent } from "@/features/workspace/application/workspaceQueries";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";
import { fileRendererCandidates } from "@/features/workspace/application/fileRenderer";
import { type FileReadPaging } from "@/features/workspace/ui/files/fileReadWindow";

function parseFileLineNumber(value: string): number | null {
  const text = value.trim();
  if (!/^[0-9\uFF10-\uFF19]+$/.test(text)) return null;
  const line = Number(text.normalize("NFKC"));
  return Number.isSafeInteger(line) && line > 0 ? line : null;
}

const styles = stylex.create({
  edge: { display: "flex", justifyContent: "center", paddingBlock: space.s1_5 },
  goto: {
    display: "flex",
    width: "220px",
    flexDirection: "column",
    gap: space.s2,
    padding: space.s2,
  },
  gotoError: { margin: 0, color: color.negative, overflowWrap: "anywhere" },
  gotoField: { flexGrow: 1, flexBasis: "6rem", minWidth: 0 },
  gotoRow: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s2 },
  jumpStatus: { display: "inline-block", maxWidth: "100%" },
  preview: { display: "contents" },
});

function FileEdgeLoader({
  pending,
  disabled,
  label,
  onLoad,
}: {
  pending: boolean;
  disabled: boolean;
  label: string;
  onLoad: (event: MouseEvent<HTMLElement>) => void;
}) {
  const t = useT();
  return (
    <div {...stylex.props(styles.edge)}>
      {pending ? (
        <span aria-hidden="true" {...stylex.props(typeStep.uiMd, vocab.muted)}>
          {t("common.loading")}
        </span>
      ) : (
        <TextButton tone="accent" size="sm" disabled={disabled} onClick={onLoad}>
          {label}
        </TextButton>
      )}
    </div>
  );
}

// Set only while FileView presents the source, including as a renderer's fallback;
// rendered content keeps the browser's formatted copy and editable fields stay native.
export function useFileReaderInput(content: string | undefined) {
  const source = useRef<HTMLDivElement>(null);
  const selectionContentRef = useRef<Element | null>(null);
  const sourceScrollRef = useRef<HTMLElement | null>(null);
  const displayed = useRef<HTMLDivElement>(null);
  const onKeyDown: KeyboardEventHandler<HTMLDivElement> = (event) => {
    const sourceView = source.current;
    const sourceContent = sourceView ? selectionContentRef.current : null;
    const selectable = sourceContent ?? sourceView ?? displayed.current;
    if (selectable) selectAllInReader(event, selectable);
    const sourceScroll = sourceView ? sourceScrollRef.current : null;
    if (
      (event.key === "ArrowLeft" || event.key === "ArrowRight") &&
      event.cancelable &&
      !isImeKey(event.nativeEvent) &&
      sourceScroll &&
      sourceScroll.scrollWidth > sourceScroll.clientWidth
    ) {
      // The named viewport holds focus while Pierre's raw column pans.
      scrollWithHorizontalArrow(event, (delta) => {
        sourceScroll.scrollBy({ left: delta, behavior: "instant" });
      });
    }
    scrollReaderWithForeignSelection(event, "both");
  };
  const onCopy: ClipboardEventHandler<HTMLDivElement> = (event) => {
    if (source.current && content !== undefined)
      copyReaderSelection(event, content, source.current);
  };
  return { source, selectionContentRef, sourceScrollRef, displayed, onKeyDown, onCopy };
}

// A reading control (retry, mode switch, rendered preview) can retire while the
// same page stays visible; focus it held returns to the reader, not the body.
export function useRetiringControlFocus(reader: RefObject<HTMLDivElement | null>, active: boolean) {
  const retiredFocusRoot = useRef<HTMLDivElement>(null);
  const controlRef = useCallback((root: HTMLDivElement | null) => {
    if (!root) return;
    if (retiredFocusRoot.current === root) retiredFocusRoot.current = null;
    return () => {
      // The retiring subtree still contains the focus owner before React removes it.
      if (root.contains(root.ownerDocument.activeElement)) retiredFocusRoot.current = root;
    };
  }, []);
  useLayoutEffect(() => {
    const root = retiredFocusRoot.current;
    if (!root) return;
    retiredFocusRoot.current = null;
    const port = reader.current;
    if (
      active &&
      !root.isConnected &&
      port?.isConnected &&
      port.ownerDocument.activeElement === port.ownerDocument.body
    ) {
      port.focus({ preventScroll: true });
    }
  });
  return controlRef;
}

export function FileJumpStatus({
  label,
  rejected,
}: {
  label: string | undefined;
  rejected: boolean;
}) {
  return (
    <>
      {label && " "}
      <span
        role="status"
        {...stylex.props(styles.jumpStatus, rejected ? vocab.negative : vocab.accent)}
      >
        {label}
      </span>
    </>
  );
}

// `fromEnd` names the edge whose page is pending or failed.
export function FileReadingBody({
  file,
  path,
  cwd,
  targetLine,
  intent,
  reader: { source, selectionContentRef, sourceScrollRef, displayed },
  renderer,
  previewRef,
  paging,
}: {
  file: WorkspaceFileContent;
  path: string;
  cwd: string | undefined;
  targetLine: number;
  intent: WorkspaceFileViewer | null;
  reader: Omit<ReturnType<typeof useFileReaderInput>, "onKeyDown" | "onCopy">;
  renderer: ReturnType<typeof fileRendererCandidates>[number] | undefined;
  previewRef: Ref<HTMLDivElement>;
  paging: (FileReadPaging & { failed: boolean }) | undefined;
}) {
  const t = useT();
  const recorded = (
    <FileView
      ref={source}
      selectionContentRef={selectionContentRef}
      sourceScrollRef={sourceScrollRef}
      path={path}
      content={file.content}
      startLine={file.startLine}
      endLine={file.endLine}
      targetLine={targetLine}
      intent={intent}
    />
  );
  const Renderer = renderer?.component;
  return (
    <div ref={displayed}>
      {paging && paging.earlier > 0 && (
        <FileEdgeLoader
          pending={paging.pending && paging.fromEnd}
          disabled={paging.unavailable || (paging.failed && paging.fromEnd)}
          label={t("file.loadEarlier", { count: paging.earlier })}
          onLoad={paging.loadEarlier}
        />
      )}
      {renderer && Renderer ? (
        <div ref={previewRef} {...stylex.props(styles.preview)}>
          <PluginBoundary
            key={renderer.id}
            plugin={renderer.id}
            label={renderer.title}
            fallback={recorded}
          >
            <Renderer cwd={cwd} path={path} {...file} fallback={recorded} />
          </PluginBoundary>
        </div>
      ) : (
        recorded
      )}
      {paging && paging.later > 0 && (
        <FileEdgeLoader
          pending={paging.pending && !paging.fromEnd}
          disabled={paging.unavailable || (paging.failed && !paging.fromEnd)}
          label={t("file.loadLater", { count: paging.later })}
          onLoad={paging.loadLater}
        />
      )}
    </div>
  );
}

export function GoToLine({ onGo }: { onGo: (line: number) => void }) {
  const t = useT();
  const errorId = useId();
  const field = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [attempted, setAttempted] = useState(false);
  const line = parseFileLineNumber(value);
  const invalid = attempted && line === null;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (line === null) {
      setAttempted(true);
      field.current?.focus();
      return;
    }
    onGo(line);
    setOpen(false);
    setValue("");
    setAttempted(false);
  };
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        render={
          <IconButton icon={iconGlyph("crosshair")} size="sm" aria-label={t("file.goToLine")} />
        }
      />
      <Popover.Content align="end" sideOffset={6}>
        <form onSubmit={submit} {...stylex.props(styles.goto)}>
          <div {...stylex.props(styles.gotoRow)}>
            <TextField
              ref={field}
              inputMode="numeric"
              aria-label={t("file.goToLine")}
              aria-describedby={invalid ? errorId : undefined}
              invalid={invalid}
              placeholder={t("file.goToLine.placeholder")}
              value={value}
              onChange={(event) => setValue(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== "Enter") return;
                // Implicit submission would bypass the IME guard, so Enter always routes here.
                event.preventDefault();
                if (!isImeKey(event.nativeEvent)) event.currentTarget.form?.requestSubmit();
              }}
              styles={styles.gotoField}
            />
            <Button type="submit" variant="primary">
              {t("file.goToLine.submit")}
            </Button>
          </div>
          {invalid && (
            <p id={errorId} {...stylex.props(typeStep.uiSm, styles.gotoError)}>
              {t("file.goToLine.invalid")}
            </p>
          )}
        </form>
      </Popover.Content>
    </Popover.Root>
  );
}
