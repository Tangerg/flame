import { useCallback, useEffect, useMemo, useRef, useState, type Ref } from "react";
import { createPortal } from "react-dom";
import { LexicalComposer } from "@lexical/react/LexicalComposer";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { ContentEditable } from "@lexical/react/LexicalContentEditable";
import { PlainTextPlugin } from "@lexical/react/LexicalPlainTextPlugin";
import { HistoryPlugin } from "@lexical/react/LexicalHistoryPlugin";
import { OnChangePlugin } from "@lexical/react/LexicalOnChangePlugin";
import { LexicalErrorBoundary } from "@lexical/react/LexicalErrorBoundary";
import {
  LexicalTypeaheadMenuPlugin,
  MenuOption,
  type MenuTextMatch,
} from "@lexical/react/LexicalTypeaheadMenuPlugin";
import {
  $createTextNode,
  $getSelection,
  $isRangeSelection,
  COMMAND_PRIORITY_LOW,
  KEY_DOWN_COMMAND,
  PASTE_COMMAND,
} from "lexical";
import { isImeKey, useContentVisible, Button, ButtonPrimitive } from "@flame/ui";
import { useComposer } from "@/features/composer/public/composer";
import { useRuntime } from "@/features/connection/public/runtime";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { resolvedWorkspaceCwd, useActiveSessionWorkspace } from "@/features/agent/public/session";
import { useWorkspaceListFiles } from "@/features/workspace/public/queries";
import { useSlashCommands, COMPOSER_KEY_BINDING } from "@/features/composer/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { submitComposer } from "@/features/composer/public/submit";
import {
  composerKeyBindingKey,
  composerPasteIntent,
} from "@/features/composer/application/composerInputEvents";
import { imageFiles } from "@/features/composer/public/input";
import { useT } from "@/lib/i18n/index";
import type { AgentInput } from "@/features/agent/public/input";
import { useNotifications } from "@/features/notifications/public/notifier";
import * as stylex from "@stylexjs/stylex";
import { color, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";

import {
  $createFileReference,
  $restoreContent,
  COMPOSER_NODES,
  readContent,
} from "@/features/composer/adapters/composerDocument";
import { type DraftContent } from "@/features/composer/domain/draft";
const styles = stylex.create({
  host: { position: "relative" },
  placeholder: {
    position: "absolute",
    insetBlockStart: 0,
    pointerEvents: "none",
    color: color.fgMuted,
  },
  editor: {
    minHeight: "1.5lh",
    maxHeight: "8lh",
    overflowY: "auto",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  },
  menu: {
    position: "relative",
    zIndex: "var(--layer-popup)",
    maxHeight: "18rem",
    overflow: "auto",
    minWidth: "14rem",
    maxWidth: "min(30rem, 90vw)",
    backgroundColor: surface.surface2,
    borderRadius: radius.floatingPanel,
    boxShadow: "var(--shadow-overlay)",
    padding: space.s1,
  },
  option: {
    display: "block",
    width: "100%",
    textAlign: "start",
    padding: space.s2,
    borderRadius: radius.sm,
    color: color.fg,
    backgroundColor: "transparent",
    borderWidth: 0,
  },
  selected: { backgroundColor: surface.surface3 },
});
class Candidate extends MenuOption {
  constructor(
    readonly kind: "file" | "slash",
    readonly label: string,
  ) {
    super(kind + ":" + label);
  }
}
function Suggestions() {
  const t = useT();
  const [editor] = useLexicalComposerContext();
  const [match, setMatch] = useState<{ query: string; kind: "file" | "slash" } | null>(null);
  const pendingKind = useRef<"file" | "slash">("file");
  const query = match?.query ?? null;
  const kind = match?.kind ?? "file";
  const setQuery = useCallback(
    (query: string | null) =>
      setMatch((previous) =>
        query === null
          ? null
          : previous?.query === query && previous.kind === pendingKind.current
            ? previous
            : { query, kind: pendingKind.current },
      ),
    [],
  );
  const trigger = useCallback((text: string): MenuTextMatch | null => {
    const match = /(?:^|\s)(@[^\s@]*|\/[^\s/]*)$/.exec(text);
    if (!match) return null;
    const token = match[1]!;
    pendingKind.current = token.startsWith("@") ? "file" : "slash";
    return {
      leadOffset: text.length - token.length,
      matchingString: token.slice(1),
      replaceableString: token,
    };
  }, []);
  const cwd = resolvedWorkspaceCwd(useActiveSessionWorkspace());
  const commands = useSlashCommands();
  const listing = useWorkspaceListFiles(
    query !== null && kind === "file" && cwd !== undefined
      ? { cwd, recursive: true, nameQuery: query, limit: 8 }
      : undefined,
  );
  const options = useMemo(
    () =>
      query === null
        ? []
        : kind === "file"
          ? (listing.data?.data ?? []).map((entry) => new Candidate("file", entry.path))
          : commands
              .filter(({ cmd }) => cmd.toLowerCase().startsWith("/" + query.toLowerCase()))
              .map(({ cmd }) => new Candidate("slash", cmd)),
    [query, kind, listing.data, commands],
  );
  return (
    <LexicalTypeaheadMenuPlugin<Candidate>
      options={options}
      onQueryChange={setQuery}
      triggerFn={trigger}
      onSelectOption={(option, node, close) => {
        editor.update(() => {
          const replacement =
            option.kind === "file"
              ? $createFileReference(option.label)
              : $createTextNode(option.label);
          if (node) {
            node.replace(replacement);
            replacement.selectEnd();
          } else {
            const selection = $getSelection();
            if ($isRangeSelection(selection)) selection.insertNodes([replacement]);
          }
          const selection = $getSelection();
          if ($isRangeSelection(selection)) selection.insertText(" ");
        });
        close();
      }}
      menuRenderFn={(
        anchor,
        { selectedIndex, setHighlightedIndex, selectOptionAndCleanUp, options: items },
      ) =>
        anchor.current
          ? createPortal(
              <div {...stylex.props(styles.menu, typeStep.uiSm)}>
                <div
                  role="listbox"
                  aria-label={t(
                    kind === "file" ? "composer.mention.heading" : "composer.slash.heading",
                  )}
                >
                  {items.map((option, index) => (
                    <ButtonPrimitive
                      type="button"
                      role="option"
                      aria-selected={selectedIndex === index}
                      tabIndex={-1}
                      key={option.key}
                      ref={(node) => option.setRefElement(node)}
                      onMouseDown={(event) => event.preventDefault()}
                      onMouseEnter={() => setHighlightedIndex(index)}
                      onClick={() => selectOptionAndCleanUp(option)}
                      {...stylex.props(styles.option, selectedIndex === index && styles.selected)}
                    >
                      {option.label}
                    </ButtonPrimitive>
                  ))}
                </div>
                {kind === "file" && items.length === 0 && (
                  <div role="status">
                    {cwd === undefined ? (
                      t("composer.mention.status.no-workspace")
                    ) : listing.isPending ? (
                      t("common.loading")
                    ) : listing.isError ? (
                      <>
                        <span>{listing.error.message}</span>
                        <Button
                          onClick={() => {
                            void listing.refetch();
                            editor.focus();
                          }}
                        >
                          {t("common.retry")}
                        </Button>
                      </>
                    ) : (
                      t("composer.mention.status.empty")
                    )}
                  </div>
                )}
              </div>,
              anchor.current,
            )
          : null
      }
    />
  );
}
function Commands({ onSend, onAddImages, onAddPaste, acceptsImages }: EditorProps) {
  const [editor] = useLexicalComposerContext();
  const access = useComposer();
  const runtime = useRuntime();
  const visible = useContentVisible();
  const { messages } = useNotifications();
  const t = useT();
  const revision = useRef<object>({});
  const lastContent = useRef<DraftContent | null>(null);
  useEffect(() => {
    editor.setEditable(visible);
  }, [editor, visible]);
  useEffect(() => {
    let release = () => {};
    const stop = editor.registerRootListener((root) => {
      release();
      if (!root) return;
      const releaseEditor = access.registerEditor({
        element: root,
        restore: (content) => {
          if (readContent(editor.getEditorState()).equals(content)) return;
          editor.update(() => $restoreContent(content), { discrete: true });
        },
        focus: () => editor.focus(),
        revision: () => revision.current,
      });
      const releaseFocus = access.registerFocusTarget(root);
      release = () => {
        releaseFocus();
        releaseEditor();
      };
    });
    return () => {
      stop();
      release();
    };
  }, [editor, access]);
  useEffect(
    () =>
      editor.registerCommand(
        KEY_DOWN_COMMAND,
        (event) => {
          if (!visible || editor.isComposing() || isImeKey(event)) return false;
          const binding = lookupExtensionByKey(COMPOSER_KEY_BINDING, composerKeyBindingKey(event));
          if (!binding) return false;
          const handled = binding.handler({
            submit: () =>
              submitComposer({
                draft: access.captureSubmission(),
                sendInput: onSend,
                canSend: () => runtimeCommandsAvailable(runtime),
                restoreFocus: access.captureFocus(),
              }),
            event,
          });
          if (handled) event.preventDefault();
          return handled === true;
        },
        COMMAND_PRIORITY_LOW,
      ),
    [editor, access, runtime, visible, onSend],
  );
  useEffect(
    () =>
      editor.registerCommand(
        PASTE_COMMAND,
        (event) => {
          if (!(event instanceof ClipboardEvent)) return false;
          const files = imageFiles(event.clipboardData?.files);
          const intent = composerPasteIntent(
            files,
            event.clipboardData?.getData("text/plain") ?? "",
            acceptsImages,
          );
          if (files.length && !acceptsImages)
            messages.notify("error", t("composer.attachImage.unsupported"));
          if (intent.kind === "native") return false;
          event.preventDefault();
          if (intent.kind === "images") onAddImages(intent.files);
          if (intent.kind === "large-text") onAddPaste(intent.text);
          return true;
        },
        COMMAND_PRIORITY_LOW,
      ),
    [editor, onAddImages, onAddPaste, acceptsImages, messages, t],
  );
  return (
    <OnChangePlugin
      ignoreSelectionChange
      ignoreHistoryMergeTagChange={false}
      onChange={(state) => {
        const content = readContent(state);
        if (lastContent.current?.equals(content)) return;
        lastContent.current = content;
        revision.current = {};
        access.checkpoint(content);
      }}
    />
  );
}
interface EditorProps {
  inputRef?: Ref<HTMLDivElement>;
  onSend(input: AgentInput): boolean;
  onAddImages(files: File[]): void;
  onAddPaste(text: string): void;
  acceptsImages: boolean;
}
export function ComposerEditor({ inputRef, ...commands }: EditorProps) {
  const access = useComposer();
  const t = useT();
  const [initial] = useState(() => access.initialContent());
  return (
    <LexicalComposer
      initialConfig={{
        namespace: "FlameComposer",
        nodes: COMPOSER_NODES,
        onError: (error) => {
          throw error;
        },
        editorState: () => $restoreContent(initial),
      }}
    >
      <div {...stylex.props(styles.host)}>
        <PlainTextPlugin
          contentEditable={
            <ContentEditable
              ref={inputRef}
              aria-label={t("composer.input.label")}
              aria-placeholder={t("composer.placeholder")}
              placeholder={() => (
                <span aria-hidden="true" {...stylex.props(styles.placeholder, typeStep.prose)}>
                  {t("composer.placeholder")}
                </span>
              )}
              data-slot="composer-input"
              data-chrome-focus=""
              {...stylex.props(styles.editor, typeStep.prose)}
            />
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
      </div>
      <HistoryPlugin />
      <Commands {...commands} />
      <Suggestions />
    </LexicalComposer>
  );
}
