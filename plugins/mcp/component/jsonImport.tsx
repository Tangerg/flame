import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { FileBraces } from "@flame/icons/react";
import {
  Button,
  FormField,
  Icon,
  ModalDialog,
  RecordBody,
  TextArea,
} from "@flame/ui";
import { McpImportError, type McpImportIssue } from "./import";
import { IMPORT, type MCPSettings } from "./settings";
import { CommandFeedback } from "./feedback";
const EXAMPLE = JSON.stringify(
  {
    mcpServers: {
      "my-server": { type: "streamableHttp", url: "https://example.com/mcp" },
    },
  },
  null,
  2,
);
const ISSUE_KEYS: Record<McpImportIssue["kind"], string> = {
  json: "mcp.import.invalidJson",
  shape: "mcp.import.invalidShape",
  empty: "mcp.import.empty",
  name: "mcp.import.invalidName",
  headerConflict: "mcp.import.headerConflict",
  unsupportedTransport: "mcp.import.unsupportedTransport",
  transport: "mcp.import.missingTransport",
  command: "mcp.import.missingCommand",
  url: "mcp.import.missingUrl",
  timeout: "mcp.import.invalidTimeout",
};
export function JsonImport({
  context,
  settings,
  t,
}: {
  context: ComponentContext;
  settings: MCPSettings;
  t: Translate;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<{ invalid: boolean; text: string }>();
  const field = useRef<HTMLTextAreaElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const errorId = useId();
  const command = state.commands.get(IMPORT);
  const busy = settings.busy(IMPORT);
  const feedback = state.feedback.get(IMPORT);
  const completed = feedback?.kind === "imported";
  useLayoutEffect(() => {
    if (error) errorRef.current?.scrollIntoView({ block: "nearest" });
  }, [error]);
  const onImport = async () => {
    setError(undefined);
    try {
      await settings.importServers(context);
    } catch (error) {
      if (context.signal.aborted) return;
      setError({
        invalid: error instanceof McpImportError,
        text:
          error instanceof McpImportError
            ? t(ISSUE_KEYS[error.issue.kind], { ...error.issue })
            : failureText(error),
      });
    }
  };
  return (
    <>
      <Button
        variant="ghost"
        pending={command?.phase === "running"}
        onClick={() => setOpen(true)}
      >
        <Icon glyph={FileBraces} size="sm" />
        {t("mcp.import")}
      </Button>
      <ModalDialog
        open={open}
        onOpenChange={setOpen}
        title={t("mcp.import")}
        closeLabel={t("common.close")}
        initialFocus={field}
        onSubmit={(event) => {
          event.preventDefault();
          if (!busy && state.importText.trim() && !completed) void onImport();
        }}
        footer={
          <>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={!state.importText.trim() || busy || completed}
              pending={command?.phase === "running"}
            >
              {t(
                command?.phase === "running"
                  ? "mcp.importing"
                  : "mcp.import.confirm",
              )}
            </Button>
          </>
        }
      >
        <FormField label={t("mcp.import.config")}>
          <TextArea
            ref={field}
            font="mono"
            invalid={error?.invalid}
            pending={busy}
            value={state.importText}
            onChange={(event) => {
              setError(undefined);
              settings.setImportText(context, event.target.value);
            }}
            rows={10}
            spellCheck={false}
            autoCapitalize="none"
            autoCorrect="off"
            aria-describedby={error?.invalid ? errorId : undefined}
            placeholder={EXAMPLE}
          />
        </FormField>
        {state.importProgress && (
          <RecordBody>
            {t("mcp.import.partial", {
              count: state.importProgress.completed,
              total: state.importProgress.total,
            })}
          </RecordBody>
        )}
        {error && (
          <div
            ref={errorRef}
            id={errorId}
            role="alert"
            style={{ whiteSpace: "normal", overflowWrap: "anywhere" }}
          >
            {error.text}
          </div>
        )}
        <CommandFeedback
          context={context}
          settings={settings}
          resource={IMPORT}
          t={t}
        />
      </ModalDialog>
    </>
  );
}
