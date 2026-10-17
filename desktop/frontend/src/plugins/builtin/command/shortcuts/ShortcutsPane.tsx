import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { Kbd, TextButton } from "@/ui";
import { Button, EmptyState, SearchField, TextField } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { comboFromEvent, comboGlyph, normalizeCombo, splitCombo } from "@/lib/combo";
import {
  SHORTCUT,
  useEffectiveCommands,
  useExtensionPoint,
  useShortcutOverrides,
} from "@/plugins/sdk";
import { color, layout, space } from "@flame/ui/tokens.stylex";
import { SettingRow, SettingsGroup } from "@/plugins/builtin/settings/kit";

const sc = stylex.create({
  pane: { display: "flex", flexDirection: "column", gap: layout.contentGap },
  toolbar: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: layout.controlGap },
  keys: {
    display: "inline-flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s1,
  },
  controls: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
  },
  recorder: { width: "12em", maxWidth: "100%" },
  warn: { color: color.warning },
});

interface Row {
  id: string;
  label: string;
  combo: string | undefined;
  editable: boolean;
  overridden: boolean;
}

type Recording =
  | { commandId: string; conflict: null }
  | { commandId: string; conflict: { combo: string; with: Row } };

function Keys({ combo }: { combo: string }) {
  return (
    <span {...stylex.props(sc.keys)}>
      {splitCombo(combo).map((part, index) => (
        <Kbd key={index}>{part}</Kbd>
      ))}
    </span>
  );
}

function ShortcutRow({
  row,
  rows,
  recording,
  onRecordingChange,
  onReturnFocus,
}: {
  row: Row;
  rows: readonly Row[];
  recording: Recording | null;
  onRecordingChange(recording: Recording | null): void;
  onReturnFocus(trigger: HTMLButtonElement | null): void;
}) {
  const t = useT();
  const change = useRef<HTMLButtonElement>(null);
  const { rebind, reset } = useShortcutOverrides.getState();
  const mine = recording?.commandId === row.id ? recording : null;
  const conflict = mine?.conflict;
  const locked = recording !== null && mine === null;

  function closeRecorder(apply?: () => void) {
    onReturnFocus(change.current);
    apply?.();
    onRecordingChange(null);
  }

  function capture(event: KeyboardEvent<HTMLInputElement>) {
    if (!mine) return;
    // Tab must leave the recorder so its cancellation control stays reachable.
    if (event.key === "Tab" && !event.altKey && !event.ctrlKey && !event.metaKey) return;
    event.preventDefault();
    event.stopPropagation();
    if (mine.conflict) return;
    const combo = comboFromEvent(event);
    if (!combo) return;
    const holder = rows.find(
      (other) =>
        other.id !== row.id && other.combo !== undefined && normalizeCombo(other.combo) === combo,
    );
    if (holder) {
      onRecordingChange({ commandId: row.id, conflict: { combo, with: holder } });
      return;
    }
    closeRecorder(() => rebind(row.id, combo));
  }

  return (
    <SettingRow
      label={row.label}
      sub={
        conflict ? (
          <span role="alert" {...stylex.props(sc.warn)}>
            {t("shortcuts.conflict", { action: conflict.with.label })}
          </span>
        ) : mine ? (
          <span role="status">{t("shortcuts.recording")}</span>
        ) : row.combo ? (
          <Keys combo={row.combo} />
        ) : (
          t("shortcuts.unbound")
        )
      }
    >
      {row.editable ? (
        <div
          role="group"
          aria-label={row.label}
          onKeyDownCapture={(event) => {
            if (!mine || event.key !== "Escape") return;
            event.preventDefault();
            event.stopPropagation();
            closeRecorder();
          }}
          {...stylex.props(sc.controls)}
        >
          {mine && (
            <TextField
              readOnly
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              aria-label={t("shortcuts.editAria", { action: row.label })}
              value={comboGlyph(conflict?.combo ?? row.combo ?? "")}
              onKeyDown={capture}
              styles={sc.recorder}
            />
          )}
          {conflict?.with.editable && (
            <TextButton
              tone="accent"
              size="sm"
              onClick={() =>
                closeRecorder(() => {
                  rebind(conflict.with.id, null);
                  rebind(row.id, conflict.combo);
                })
              }
            >
              {t("shortcuts.replace")}
            </TextButton>
          )}
          <TextButton
            ref={change}
            tone="muted"
            size="sm"
            disabled={locked}
            aria-label={mine ? t("common.cancel") : t("shortcuts.editAria", { action: row.label })}
            onClick={() =>
              mine ? closeRecorder() : onRecordingChange({ commandId: row.id, conflict: null })
            }
          >
            {mine ? t("common.cancel") : t("shortcuts.edit")}
          </TextButton>
          {!mine && row.overridden && (
            <TextButton
              tone="muted"
              size="sm"
              disabled={locked}
              onClick={() => {
                onReturnFocus(change.current);
                reset(row.id);
              }}
            >
              {t("shortcuts.reset")}
            </TextButton>
          )}
        </div>
      ) : undefined}
    </SettingRow>
  );
}

export function ShortcutsPane() {
  const t = useT();
  const commands = useEffectiveCommands();
  const fixed = useExtensionPoint(SHORTCUT);
  const overrides = useShortcutOverrides((state) => state.overrides);
  const { resetAll, setRecording } = useShortcutOverrides.getState();
  const filter = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [recording, setRecordingFor] = useState<Recording | null>(null);
  const recordingActive = recording !== null;
  const focusReturn = useRef<{ trigger: HTMLButtonElement | null } | undefined>(undefined);

  useLayoutEffect(() => {
    const request = focusReturn.current;
    focusReturn.current = undefined;
    if (!request) return;
    // The update following a request can remove its trigger from combo-filtered results.
    const target = request.trigger?.isConnected ? request.trigger : filter.current;
    target?.focus();
  });

  const rows = useMemo<Row[]>(
    () =>
      [
        ...commands.map((command) => ({
          id: command.id,
          label: t(command.label),
          combo: command.combo,
          editable: true,
          overridden: Object.hasOwn(overrides, command.id),
        })),
        ...fixed
          .filter((shortcut) => shortcut.description)
          .map((shortcut) => ({
            id: `fixed:${shortcut.key}`,
            label: t(shortcut.description ?? ""),
            combo: shortcut.key,
            editable: false,
            overridden: false,
          })),
      ].sort((a, b) => a.label.localeCompare(b.label)),
    [commands, fixed, overrides, t],
  );

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter(
      (row) => row.label.toLowerCase().includes(q) || (row.combo ?? "").toLowerCase().includes(q),
    );
  }, [rows, query]);

  useEffect(() => {
    setRecording(recordingActive);
    return () => setRecording(false);
  }, [recordingActive, setRecording]);

  return (
    <div {...stylex.props(sc.pane)}>
      <div {...stylex.props(sc.toolbar)}>
        <SearchField
          ref={filter}
          size="lg"
          disabled={recordingActive}
          value={query}
          onValueChange={setQuery}
          placeholder={t("shortcuts.filter")}
          aria-label={t("shortcuts.filterAria")}
        />
        {Object.keys(overrides).length > 0 && (
          <Button
            variant="ghost"
            disabled={recordingActive}
            onClick={() => {
              focusReturn.current = { trigger: null };
              resetAll();
            }}
          >
            {t("shortcuts.resetAll")}
          </Button>
        )}
      </div>

      <SettingsGroup>
        {shown.length === 0 ? (
          <EmptyState icon={iconGlyph("search")} title={t("shortcuts.empty")} />
        ) : (
          shown.map((row) => (
            <ShortcutRow
              key={row.id}
              row={row}
              rows={rows}
              recording={recording}
              onRecordingChange={setRecordingFor}
              onReturnFocus={(trigger) => {
                focusReturn.current = { trigger };
              }}
            />
          ))
        )}
      </SettingsGroup>
    </div>
  );
}
