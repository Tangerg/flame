import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, IconButton, knownIconName, reveal, StatusDot, vocab } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
import { useT } from "@/lib/i18n";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import { toolCardActions, toolCardModel, toolCardViewOpener } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "@/ui/agent";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { toolMetaInk } from "./toolMetaInk";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const tc = stylex.create({
  full: { width: "100%" },
  sans: { fontFamily: "var(--font-sans)" },
  meta: { fontWeight: weight.medium },
  status: {
    display: { default: "none", "@container (min-width: 24rem)": "flex" },
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1_5,
  },
  failure: { flexShrink: 0, color: color.negative },
  failureLine: {
    marginTop: space.s0_5,
    // Lands on the LABEL: the 16px mark plus the trigger's own 6px gap.
    marginLeft: "calc(var(--spacing) * 5.5)",
    overflowWrap: "anywhere",
    // Normal ink, not the tone: the "failed" word beside the row already carries that, and the
    // timeline inks the same sentence the same way. A whole red paragraph is a shout.
    color: color.fg,
  },
});

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const allActions = useExtensionPoint(TOOL_ACTION);
  const allViewOpeners = useExtensionPoint(TOOL_VIEW_OPENER);
  const actions = toolCardActions(tool, allActions);
  const viewOpener = toolCardViewOpener(tool, allViewOpeners);
  const onOpenView = viewOpener
    ? () => {
        void Promise.resolve(viewOpener.open(tool)).catch((err) => {
          const owner = lookupToolViewOpenerOwner(viewOpener.id) ?? "unknown";
          console.error(`[plugin] tool view opener ${viewOpener.id} threw:`, err);
          reportPluginError(owner, "command", err, `tool view opener: ${viewOpener.id}`);
        });
      }
    : undefined;

  return (
    <>
      <AgentActivityDisclosure
        data-tool={tool.name}
        icon={toolCallIconFor(tool)}
        // Every invocation stays on the work-narrative line and takes the neutral tone,
        // whatever its safety class or outcome: the material result earns a surface only once
        // the row is opened, and colouring the identity glyph turns a failure or a refusal
        // back into a status card.
        shell="line"
        contentInset="rows"
        label={<ToolText value={model.intent.label} styles={tc.full} />}
        detail={model.detail ? <ToolText value={model.detail} styles={tc.full} /> : undefined}
        trailing={
          <>
            {model.diffStat && (
              <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
            )}
            <ToolMeta items={model.metaItems} />
            {model.running && <StatusDot tone="running" />}
            {model.error !== undefined && (
              <span
                data-slot="tool-status"
                data-tone="negative"
                {...stylex.props(tc.sans, tc.failure, typeStep.uiXs)}
              >
                {t("tool.state.failed")}
              </span>
            )}
            {model.denied && (
              <span data-slot="tool-status" {...stylex.props(tc.sans, vocab.muted, typeStep.uiXs)}>
                {t("tool.state.denied")}
              </span>
            )}
          </>
        }
        // An ARRAY, never a fragment: the slot renders on `Children.count`, and a fragment counts
        // as one child however empty it is — which would hang ten pixels of padding off the right
        // of every row that has no action at all.
        actions={[
          ...actions.map((action) => (
            <IconButton
              key={action.id}
              data-reveal="hover"
              icon={knownIconName(action.icon) ?? "tool"}
              size="xs"
              quiet
              title={t(action.title)}
              onClick={(event) => {
                event.stopPropagation();
                void Promise.resolve(action.run(tool)).catch((err) => {
                  const owner = lookupToolActionOwner(action.id) ?? "unknown";
                  console.error(`[plugin] tool action ${action.id} threw:`, err);
                  reportPluginError(owner, "command", err, `tool action: ${action.id}`);
                });
              }}
              className={stylex.props(reveal.shown).className}
            />
          )),
          ...(onOpenView
            ? [
                <IconButton
                  key="open-view"
                  data-reveal="hover"
                  data-slot="tool-open-view"
                  icon="panel-r"
                  size="xs"
                  quiet
                  title={t("workspace.view.openBeside")}
                  onClick={(event) => {
                    event.stopPropagation();
                    onOpenView();
                  }}
                  className={stylex.props(reveal.shown).className}
                />,
              ]
            : []),
        ]}
        open={expanded}
        onToggle={onToggleExpand}
      >
        {/* A refused call never ran, so there is no result to disclose. It used to open on a
          preview that said "No changes to show" — which the header, reading `denied`, had
          already said, and which a chevron had promised was worth a click. */}
        {model.denied ? undefined : <ToolPreview tool={tool} />}
      </AgentActivityDisclosure>
      {/* UNDER the row, not inside its disclosure and not in its detail slot.
          The detail is one truncating line, so an error there hid the subject and still could
          not be read; the disclosure is shut by default, so an error there is a failure behind
          a chevron. Here it wraps, it is selectable, and it is on screen without a click. */}
      {model.error !== undefined && (
        <p data-slot="tool-error" {...stylex.props(tc.failureLine, typeStep.uiSm)}>
          {model.error}
        </p>
      )}
    </>
  );
}

function ToolMeta({ items }: { items: ToolMetaItem[] }) {
  if (items.length === 0) return null;

  return (
    <span {...stylex.props(tc.status)}>
      {items.map((item) => (
        <span
          key={item.id}
          data-tone={item.tone}
          {...stylex.props(tc.meta, toolMetaInk.card[item.tone], typeStep.uiXs, face.mono)}
        >
          {item.label}
        </span>
      ))}
    </span>
  );
}
