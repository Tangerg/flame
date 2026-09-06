import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useRef, useState, useSyncExternalStore } from "react";
import { useMCPServers } from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { MCP_SERVERS_PANE } from "@/plugins/builtin/settings/kit/panes";
import {
  Badge,
  Collapsible,
  DataView,
  Icon,
  knownIconName,
  PillButton,
  Pressable,
  SectionLabel,
  TextArea,
  TextButton,
  Well,
} from "@/ui";
import { McpRow } from "./views/McpRow";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import {
  type DiagnosticArgumentsParseResult,
  diagnosticToolMaterialGeneration,
  formatDiagnosticToolResult,
  invokeDiagnosticTool,
  parseDiagnosticToolArguments,
  subscribeDiagnosticToolMaterialGeneration,
} from "@/plugins/builtin/workspace/application/diagnosticTool";
import {
  type BuiltinToolRowViewModel,
  builtinToolCatalogViewModel,
  toolCatalogSubtext,
  toolCatalogViewModel,
  useBuiltinToolConfigs,
} from "@/plugins/builtin/workspace/application/toolCatalog";

function SectionHead({ children, count }: { children: React.ReactNode; count?: number }) {
  return (
    <SectionLabel
      className="px-[var(--density-column-gutter-wide)] pb-1 pt-2"
      trailing={count === undefined ? undefined : <span className="font-mono">{count}</span>}
    >
      {children}
    </SectionLabel>
  );
}

function BuiltinToolsSection() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const workspaceKey =
    workspace.status === "ready"
      ? (workspace.cwd ?? "default")
      : `resolving:${workspace.sessionId}`;
  const { data, isLoading } = useBuiltinToolConfigs();
  const view = builtinToolCatalogViewModel(data ?? []);
  if (isLoading || view.isEmpty) return null;
  return (
    <div className="pb-1.5">
      <p className="px-[var(--density-column-gutter-wide)] pb-2 text-ui-xs leading-body text-fg-muted">
        {t("tools.diagnostics.sub")}
      </p>
      {view.families.map((family) => (
        <div key={family.id} className="pb-1">
          <SectionHead count={family.rows.length}>{t(family.titleKey)}</SectionHead>
          {family.rows.map((tool) => (
            <DiagnosticToolRow
              key={`${workspaceKey}:${tool.id}`}
              tool={tool}
              cwd={cwd}
              enabled={workspace.status === "ready"}
            />
          ))}
        </div>
      ))}
      <SectionHead>{t("tools.mcp")}</SectionHead>
    </div>
  );
}

export function DiagnosticToolRow(props: {
  tool: BuiltinToolRowViewModel;
  cwd?: string;
  enabled: boolean;
}) {
  const generation = useSyncExternalStore(
    subscribeDiagnosticToolMaterialGeneration,
    diagnosticToolMaterialGeneration,
    diagnosticToolMaterialGeneration,
  );
  return <DiagnosticToolRowPresentation materialGeneration={generation} {...props} />;
}

type DiagnosticArgumentsError = Extract<DiagnosticArgumentsParseResult, { ok: false }>["reason"];

function DiagnosticToolRowPresentation({
  tool,
  cwd,
  enabled,
  materialGeneration,
}: {
  tool: BuiltinToolRowViewModel;
  cwd?: string;
  enabled: boolean;
  materialGeneration: bigint;
}) {
  const t = useT();
  const panelId = useId();
  const [open, setOpen] = useState(false);
  const [argumentsText, setArgumentsText] = useState("{}");
  const [argumentsError, setArgumentsError] = useState<DiagnosticArgumentsError | null>(null);

  return (
    <div className="flex flex-col">
      <Pressable
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={t(open ? "tools.diagnostics.collapse" : "tools.diagnostics.expand", {
          tool: tool.name,
        })}
        onClick={() => setOpen((value) => !value)}
        className="grid grid-cols-[14px_auto_minmax(0,1fr)] items-start gap-2.5 px-[var(--density-column-gutter-wide)] py-1 text-left hover:bg-hover"
      >
        <Icon
          name="chevron-down"
          size="xs"
          className={cn("mt-1 text-fg-faint transition-transform", !open && "-rotate-90")}
        />
        <Icon name={knownIconName(tool.icon) ?? "tool"} size="xs" className="mt-1 text-fg-faint" />
        <span className="min-w-0">
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="truncate font-mono text-ui-sm text-fg">{tool.name}</span>
            {tool.safety && (
              <Badge tone={tool.safety.tone} className="font-mono">
                {tool.safety.label}
              </Badge>
            )}
          </span>
          <span className="block truncate text-ui-xs text-fg-faint" title={tool.description}>
            {tool.description}
          </span>
        </span>
      </Pressable>
      <Collapsible open={open}>
        <DiagnosticToolInvocationMaterial
          key={materialGeneration.toString()}
          tool={tool}
          cwd={cwd}
          enabled={enabled}
          panelId={panelId}
          argumentsText={argumentsText}
          argumentsError={argumentsError}
          onArgumentsTextChange={setArgumentsText}
          onArgumentsError={setArgumentsError}
        />
      </Collapsible>
    </div>
  );
}

function DiagnosticToolInvocationMaterial({
  tool,
  cwd,
  enabled,
  panelId,
  argumentsText,
  argumentsError,
  onArgumentsTextChange,
  onArgumentsError,
}: {
  tool: BuiltinToolRowViewModel;
  cwd?: string;
  enabled: boolean;
  panelId: string;
  argumentsText: string;
  argumentsError: DiagnosticArgumentsError | null;
  onArgumentsTextChange: (value: string) => void;
  onArgumentsError: (error: DiagnosticArgumentsError | null) => void;
}) {
  const t = useT();
  const runningRef = useRef(false);
  const [running, setRunning] = useState(false);
  const [runtimeError, setRuntimeError] = useState<string | null>(null);
  const [result, setResult] = useState<string | null>(null);
  const schema = JSON.stringify(tool.parameters, null, 2);
  const error = argumentsError ? t(`tools.diagnostics.error.${argumentsError}`) : runtimeError;

  const invoke = async (): Promise<void> => {
    if (!enabled || runningRef.current) return;
    const parsed = parseDiagnosticToolArguments(argumentsText);
    if (!parsed.ok) {
      onArgumentsError(parsed.reason);
      return;
    }

    runningRef.current = true;
    setRunning(true);
    onArgumentsError(null);
    setRuntimeError(null);
    setResult(null);
    try {
      const value = await invokeDiagnosticTool({
        name: tool.name,
        arguments: parsed.value,
        ...(cwd ? { cwd } : {}),
      });
      setResult(formatDiagnosticToolResult(value));
    } catch (cause) {
      if (!wasGenerationRetired(cause)) {
        setRuntimeError(rpcErrorText(cause) ?? t("tools.diagnostics.error.invoke"));
      }
    } finally {
      runningRef.current = false;
      setRunning(false);
    }
  };

  return (
    <div
      id={panelId}
      className="flex flex-col gap-2.5 px-[var(--density-column-gutter-wide)] pt-1 pb-3 pl-[58px]"
    >
      <label className="flex flex-col gap-1 text-ui-xs font-medium text-fg-muted">
        {t("tools.diagnostics.arguments")}
        <TextArea
          value={argumentsText}
          rows={5}
          font="mono"
          invalid={argumentsError !== null}
          aria-invalid={argumentsError !== null}
          disabled={running}
          spellCheck={false}
          onChange={(event) => {
            onArgumentsTextChange(event.target.value);
            onArgumentsError(null);
            if (runtimeError) setRuntimeError(null);
          }}
        />
      </label>
      <div>
        <span className="text-ui-xs font-medium text-fg-muted">
          {t("tools.diagnostics.schema")}
        </span>
        <Well cap="sm" className="mt-1">
          {schema}
        </Well>
      </div>
      <div className="flex items-center gap-2">
        <PillButton
          size="sm"
          variant="accent"
          disabled={!enabled || running}
          onClick={() => void invoke()}
        >
          {running ? t("tools.diagnostics.running") : t("tools.diagnostics.run")}
        </PillButton>
        {error && (
          <span className="text-ui-xs text-negative" aria-live="polite">
            {error}
          </span>
        )}
      </div>
      {result !== null && (
        <div>
          <span className="text-ui-xs font-medium text-fg-muted">
            {t("tools.diagnostics.result")}
          </span>
          <Well cap="md" className="mt-1" aria-live="polite">
            {result}
          </Well>
        </div>
      )}
    </div>
  );
}

function openMcpSettings(): void {
  openWorkspaceSettingsPane(MCP_SERVERS_PANE);
}

export function ToolsTab() {
  const t = useT();
  const { data, isLoading, isError, refetch } = useMCPServers();
  const view = toolCatalogViewModel(data ?? []);

  return (
    <WorkspaceViewLayout
      icon="tool"
      titleStrong
      title="tools.title"
      sub={toolCatalogSubtext(t, view)}
      scrollClassName="py-1"
    >
      <BuiltinToolsSection />
      <DataView
        items={view.mcpServers}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={4}
        empty={{
          icon: "tool",
          title: t("tools.empty.title"),
          sub: t("tools.empty.sub"),
        }}
      >
        {(rows) => rows.map((s) => <McpRow key={s.id} server={s} />)}
      </DataView>
      <TextButton
        size="sm"
        onClick={openMcpSettings}
        className="px-[var(--density-column-gutter-wide)] pt-3.5 pb-4.5 leading-body"
      >
        <Icon name="settings" size="xs" />
        {t("tools.footer")}
      </TextButton>
    </WorkspaceViewLayout>
  );
}
