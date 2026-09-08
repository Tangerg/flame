import * as stylex from "@stylexjs/stylex";
import type { PluginError, PluginErrorSource } from "@/plugins/sdk";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useState } from "react";
import { Badge, Icon, IconButton, PillButton, TextButton } from "@/ui";
import { copyText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { useInstalledPlugins, usePluginErrorStore } from "@/plugins/sdk";
import {
  color,
  face,
  leading,
  radius,
  space,
  surface,
  type as typeStep,
} from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const pp = stylex.create({
  plugin: {
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
  },
  // A plugin that failed to load keeps its wash whether or not the pointer is on it.
  faulted: { backgroundColor: surface.negativeWash },
  head: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    gap: space.s2_5,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
  },
  errors: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    paddingInline: space.s3,
    paddingBottom: space.s3,
  },
  errorCard: {
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
  },
  errorHead: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s2,
  },
  // A trace scrolls rather than growing the pane: it is evidence, not the point of the row.
  stack: {
    marginTop: space.s1_5,
    maxHeight: "calc(var(--spacing) * 56)",
    overflow: "auto",
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    fontFamily: "var(--font-mono)",
    lineHeight: leading.body,
    color: color.fgMuted,
  },
});

export function PluginsPane() {
  const t = useT();
  const installed = useInstalledPlugins();
  const log = usePluginErrorStore((s) => s.log);
  const clearFor = usePluginErrorStore((s) => s.clearFor);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  const errorsByPlugin = new Map<string, PluginError[]>();
  for (const err of log) {
    const list = errorsByPlugin.get(err.plugin);
    if (list) list.unshift(err);
    else errorsByPlugin.set(err.plugin, [err]);
  }

  const rows = [...installed].sort((a, b) => {
    const ea = errorsByPlugin.get(a)?.length ?? 0;
    const eb = errorsByPlugin.get(b)?.length ?? 0;
    if (ea !== eb) return eb - ea;
    return a.localeCompare(b);
  });

  const toggle = (name: string) =>
    setExpanded((cur) => {
      const next = new Set(cur);
      if (!next.delete(name)) next.add(name);
      return next;
    });

  return (
    <div>
      <div {...stylex.props(ss.stackTight)}>
        {rows.map((name) => {
          const errors = errorsByPlugin.get(name) ?? [];
          const errCount = errors.length;
          const open = expanded.has(name);
          return (
            <div key={name} {...stylex.props(pp.plugin, errCount > 0 && pp.faulted)}>
              <div {...stylex.props(pp.head)}>
                <div>
                  <div {...stylex.props(ss.label, typeStep.uiMd)}>{name}</div>
                  {errCount > 0 && (
                    <TextButton
                      tone="negative"
                      onClick={() => toggle(name)}
                      title={open ? t("plugins.errorDetail.hide") : t("plugins.errorDetail.show")}
                      {...stylex.props(ss.afterLine)}
                    >
                      <Icon name="bug" size="xs" />
                      {t("plugins.errors", { count: errCount })}
                      <Icon name={open ? "chevron-up" : "chevron-down"} size="xs" />
                    </TextButton>
                  )}
                </div>
                <div {...stylex.props(ss.lineTight)}>
                  {errCount > 0 && (
                    <PillButton variant="outlined" size="sm" onClick={() => clearFor(name)}>
                      {t("plugins.clear")}
                    </PillButton>
                  )}
                </div>
              </div>
              {open && errCount > 0 && (
                <div {...stylex.props(pp.errors)}>
                  {errors.map((err) => (
                    <ErrorEntry key={err.id} err={err} />
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// The badge names WHICH of a plugin's contracts broke, so it is read by a person and
// belongs in the catalogue: four of these matched their wire value only by coincidence.
const SOURCE_LABEL_KEYS: Record<PluginErrorSource, string> = {
  setup: "plugins.errorSource.setup",
  render: "plugins.errorSource.render",
  events: "plugins.errorSource.events",
  command: "plugins.errorSource.command",
  other: "plugins.errorSource.other",
};

function ErrorEntry({ err }: { err: PluginError }) {
  const t = useT();
  const time = formatClock(err.timestamp);
  const source = t(SOURCE_LABEL_KEYS[err.source]);
  const copy = () =>
    void copyText(`[${source}] ${err.message}${err.detail ? `\n\n${err.detail}` : ""}`);
  return (
    <div {...stylex.props(pp.errorCard)}>
      <div {...stylex.props(pp.errorHead)}>
        <Badge tone="negative" face="mono">
          {source}
        </Badge>
        <span {...stylex.props(ss.truncate, ss.label, typeStep.uiMd)} title={err.message}>
          {err.message}
        </span>
        <div {...stylex.props(ss.lineTight)}>
          <span {...stylex.props(ss.faint, typeStep.uiXs, face.mono)}>{time}</span>
          <IconButton icon="copy" iconSize="xs" title={t("plugins.copyError")} onClick={copy} />
        </div>
      </div>
      {err.detail && <pre {...stylex.props(pp.stack, typeStep.uiSm)}>{err.detail}</pre>}
    </div>
  );
}
