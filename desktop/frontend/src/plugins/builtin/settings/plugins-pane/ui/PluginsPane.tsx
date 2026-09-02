import type { PluginError, PluginErrorSource } from "@/plugins/sdk";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useState } from "react";
import { Badge, Icon, IconButton, PillButton, TextButton } from "@/ui";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useInstalledPlugins, usePluginErrorStore } from "@/plugins/sdk";

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
      <div className="flex flex-col gap-2">
        {rows.map((name) => {
          const errors = errorsByPlugin.get(name) ?? [];
          const errCount = errors.length;
          const open = expanded.has(name);
          return (
            <div
              key={name}
              className={cn(
                "rounded-md transition-colors hover:bg-hover",
                errCount > 0 && "bg-negative-wash",
              )}
            >
              <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2.5 px-3 py-2.5">
                <div>
                  <div className="text-ui-md font-medium text-fg">{name}</div>
                  {errCount > 0 && (
                    <TextButton
                      tone="negative"
                      onClick={() => toggle(name)}
                      title={open ? t("plugins.errorDetail.hide") : t("plugins.errorDetail.show")}
                      className="mt-1.5"
                    >
                      <Icon name="bug" size="xs" />
                      {t("plugins.errors", { count: errCount })}
                      <Icon name={open ? "chevron-up" : "chevron-down"} size="xs" />
                    </TextButton>
                  )}
                </div>
                <div className="flex items-center gap-1.5">
                  {errCount > 0 && (
                    <PillButton variant="outlined" size="sm" onClick={() => clearFor(name)}>
                      {t("plugins.clear")}
                    </PillButton>
                  )}
                </div>
              </div>
              {open && errCount > 0 && (
                <div className="flex flex-col gap-1.5 px-3 pb-3">
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
    <div className="rounded-md bg-sunken px-2.5 py-2">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2">
        <Badge tone="negative" className="font-mono">
          {source}
        </Badge>
        <span className="truncate font-medium text-ui-md text-fg" title={err.message}>
          {err.message}
        </span>
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-ui-xs text-fg-faint">{time}</span>
          <IconButton icon="copy" iconSize="xs" title={t("plugins.copyError")} onClick={copy} />
        </div>
      </div>
      {err.detail && (
        <pre className="mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words font-mono text-ui-sm leading-body text-fg-muted">
          {err.detail}
        </pre>
      )}
    </div>
  );
}
