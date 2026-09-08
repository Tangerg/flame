import * as stylex from "@stylexjs/stylex";
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
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { type as typeStep } from "@/styles/tokens.stylex";
import { toolStyles as os, viewStyles as vs } from "./views/viewStyles";
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
      className={stylex.props(vs.gutter, os.headPad).className}
      trailing={count === undefined ? undefined : <span {...stylex.props(vs.mono)}>{count}</span>}
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
    <div {...stylex.props(os.familyList)}>
      <p {...stylex.props(vs.gutter, os.blurb, typeStep.uiXs)}>{t("tools.diagnostics.sub")}</p>
      {view.families.map((family) => (
        <div key={family.id} {...stylex.props(vs.sectionPad)}>
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
    <div {...stylex.props(vs.stack)}>
      <Pressable
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={t(open ? "tools.diagnostics.collapse" : "tools.diagnostics.expand", {
          tool: tool.name,
        })}
        onClick={() => setOpen((value) => !value)}
        className={stylex.props(os.toolRow, vs.gutter, vs.wash).className}
      >
        <Icon
          name="chevron-down"
          size="xs"
          className={stylex.props(os.rowGlyph, vs.chevron, !open && vs.chevronShut).className}
        />
        <Icon
          name={knownIconName(tool.icon) ?? "tool"}
          size="xs"
          className={stylex.props(os.rowGlyph, vs.caption).className}
        />
        <span {...stylex.props(vs.min)}>
          <span {...stylex.props(vs.lineBaseline)}>
            <span {...stylex.props(vs.ink, vs.mono, vs.truncate, typeStep.uiSm)}>{tool.name}</span>
            {tool.safety && (
              <Badge tone={tool.safety.tone} face="mono">
                {tool.safety.label}
              </Badge>
            )}
          </span>
          <span
            {...stylex.props(os.toolBlurb, vs.truncate, typeStep.uiXs)}
            title={tool.description}
          >
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
    <div id={panelId} {...stylex.props(vs.stack, os.panelGap, vs.gutter, os.panelInset)}>
      <label {...stylex.props(vs.stack, os.fieldGap, os.fieldLabel, typeStep.uiXs)}>
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
        <span {...stylex.props(os.fieldLabel, typeStep.uiXs)}>{t("tools.diagnostics.schema")}</span>
        <Well cap="sm" className={stylex.props(os.afterLabel).className}>
          {schema}
        </Well>
      </div>
      <div {...stylex.props(vs.line)}>
        <PillButton
          size="sm"
          variant="accent"
          disabled={!enabled || running}
          onClick={() => void invoke()}
        >
          {running ? t("tools.diagnostics.running") : t("tools.diagnostics.run")}
        </PillButton>
        {error && (
          <span {...stylex.props(vs.negative, typeStep.uiXs)} aria-live="polite">
            {error}
          </span>
        )}
      </div>
      {result !== null && (
        <div>
          <span {...stylex.props(os.fieldLabel, typeStep.uiXs)}>
            {t("tools.diagnostics.result")}
          </span>
          <Well cap="md" className={stylex.props(os.afterLabel).className} aria-live="polite">
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
        shape="row"
        size="sm"
        onClick={openMcpSettings}
        className={stylex.props(vs.gutter, os.footer).className}
      >
        <Icon name="settings" size="xs" />
        {t("tools.footer")}
      </TextButton>
    </WorkspaceViewLayout>
  );
}
