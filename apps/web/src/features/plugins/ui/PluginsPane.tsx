import { iconGlyph } from "@/ui/icons/index";
import { PackageManager } from "@/features/plugins/ui/PackageManager";
import * as stylex from "@stylexjs/stylex";
import { type PluginError, type PluginErrorSource } from "@/extensions/errors";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useId, useRef, useState } from "react";
import { Icon } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Button, Badge, IconButton, PanelSection, ScrollRegion, vocab } from "@flame/ui";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import { useInstalledPlugins } from "@/extensions/kernel";
import { usePluginErrorStore } from "@/extensions/errors";
import {
  color,
  face,
  leading,
  layout,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
} from "@flame/ui/tokens.stylex";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const pp = stylex.create({
  plugin: {
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  faulted: { backgroundColor: surface.negativeWash },
  name: { overflowWrap: "anywhere" },
  head: {
    display: "grid",
    gridTemplateColumns: {
      default: "minmax(0, 1fr) auto",
      "@container (max-width: 440px)": "minmax(0, 1fr)",
    },
    gap: space.s2_5,
    minWidth: 0,
    paddingInline: layout.cardPadding,
    paddingBlock: space.s3,
  },
  errors: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    paddingInline: layout.cardPadding,
    paddingBottom: space.s3,
  },
  errorCard: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    gap: space.s1_5,
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
  },
  errorHead: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s2,
  },
  errorActions: { marginInlineStart: "auto" },
  message: { margin: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  stack: {
    maxHeight: "calc(var(--spacing) * 56)",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
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
  const loadedRef = useRef<HTMLSpanElement>(null);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  const errorsByPlugin = new Map<string, PluginError[]>();
  for (const err of log) {
    const list = errorsByPlugin.get(err.plugin);
    if (list) list.unshift(err);
    else errorsByPlugin.set(err.plugin, [err]);
  }

  const rows = [...new Set([...installed, ...errorsByPlugin.keys()])].sort((a, b) => {
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
    <>
      <PackageManager />
      <PanelSection
        title={
          <span ref={loadedRef} tabIndex={-1}>
            {t("plugins.loadedTitle")}
          </span>
        }
        aria-label={t("plugins.loadedTitle")}
      >
        <SettingsGroup hidden={!rows.length}>
          {rows.map((name) => {
            const errors = errorsByPlugin.get(name) ?? [];
            const errCount = errors.length;
            const open = expanded.has(name);
            return (
              <div
                key={name}
                role="group"
                aria-label={name}
                {...stylex.props(pp.plugin, errCount > 0 && pp.faulted)}
              >
                <div {...stylex.props(pp.head)}>
                  <div {...stylex.props(vocab.min)}>
                    <div {...stylex.props(pp.name, ss.label, typeStep.uiMd)}>{name}</div>
                    {errCount > 0 && (
                      <TextButton
                        tone="negative"
                        onClick={() => toggle(name)}
                        aria-expanded={open}
                        title={open ? t("plugins.errorDetail.hide") : t("plugins.errorDetail.show")}
                        {...stylex.props(vocab.afterLine)}
                      >
                        <Icon name="bug" size="xs" />
                        {t("plugins.errors", { count: errCount })}
                        <Icon name={open ? "chevron-up" : "chevron-down"} size="xs" />
                      </TextButton>
                    )}
                  </div>
                  <div {...stylex.props(vocab.lineTight)}>
                    {errCount > 0 && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          clearFor(name);
                          loadedRef.current?.focus({ preventScroll: true });
                        }}
                      >
                        {t("plugins.clear")}
                      </Button>
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
        </SettingsGroup>
      </PanelSection>
    </>
  );
}

const SOURCE_LABEL_KEYS: Record<PluginErrorSource, string> = {
  setup: "plugins.errorSource.setup",
  render: "plugins.errorSource.render",
  events: "plugins.errorSource.events",
  command: "plugins.errorSource.command",
  other: "plugins.errorSource.other",
};

function ErrorEntry({ err }: { err: PluginError }) {
  const t = useT();
  const messageId = useId();
  const time = formatClock(err.timestamp);
  const source = t(SOURCE_LABEL_KEYS[err.source]);
  const copy = () =>
    void writeToClipboard(`[${source}] ${err.message}${err.detail ? `\n\n${err.detail}` : ""}`, {
      successLabel: t("plugins.errorCopied"),
      failureLabel: t("plugins.copyFailed"),
    });
  return (
    <div {...stylex.props(pp.errorCard)}>
      <p id={messageId} {...stylex.props(pp.message, ss.label, typeStep.uiMd)}>
        {err.message}
      </p>
      <div {...stylex.props(pp.errorHead)}>
        <Badge tone="negative" face="mono">
          {source}
        </Badge>
        <div {...stylex.props(pp.errorActions, vocab.lineTight)}>
          <span {...stylex.props(vocab.faint, typeStep.uiXs, face.mono)}>{time}</span>
          <IconButton
            icon={iconGlyph("copy")}
            size="xs"
            aria-label={t("plugins.copyError")}
            aria-describedby={messageId}
            onClick={copy}
          />
        </div>
      </div>
      {err.detail && (
        <ScrollRegion label={err.message} styles={[pp.stack, typeStep.uiSm]} data-focus-inset="">
          {err.detail}
        </ScrollRegion>
      )}
    </div>
  );
}
