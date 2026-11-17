import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Kbd } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Button, EmptyState, SearchField, TextField, useContentVisible } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { normalizeHotkey, areHotkeysEqual, formatForDisplay } from "@tanstack/hotkeys";
import { useHotkeyRecorder } from "@tanstack/react-hotkeys";
import { SHORTCUT } from "@/features/command/public/extensionPoints";
import {
  useEffectiveCommands,
  useShortcutOverrides,
} from "@/features/command/public/shortcutOverrides";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { color, layout, space } from "@flame/ui/tokens.stylex";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
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
      {formatForDisplay(normalizeHotkey(combo), { parts: true }).map((part, index) => (
        <Kbd key={index}>{part}</Kbd>
      ))}
    </span>
  );
}

function conflictingShortcut(rows: readonly Row[], commandId: string, combo: string) {
  const candidate = normalizeHotkey(combo);
  return rows.find(
    (other) =>
      other.id !== commandId &&
      other.combo !== undefined &&
      areHotkeysEqual(normalizeHotkey(other.combo), candidate),
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
  const statusId = useId();
  const { rebind, reset } = useShortcutOverrides.getState();
  const mine = recording?.commandId === row.id ? recording : null;
  const conflict = mine?.conflict;
  const displayedCombo = conflict?.combo ?? row.combo;
  const locked = recording !== null && mine === null;

  function closeRecorder(apply?: () => void) {
    onReturnFocus(change.current);
    apply?.();
    onRecordingChange(null);
  }

  const recorder = useHotkeyRecorder({
    recordBy: "auto",
    ignoreInputs: false,
    validate: (combo) => {
      const holder = conflictingShortcut(rows, row.id, combo);
      if (!holder) return true;
      onRecordingChange({ commandId: row.id, conflict: { combo, with: holder } });
      return false;
    },
    onRecord: (combo) => closeRecorder(() => rebind(row.id, combo)),
    onCancel: () => closeRecorder(),
    onClear: () => closeRecorder(() => rebind(row.id, null)),
  });
  const { startRecording, stopRecording } = recorder;
  const active = mine !== null;
  useEffect(() => {
    if (active) startRecording();
    return stopRecording;
  }, [active, startRecording, stopRecording]);

  return (
    <SettingRow
      label={row.label}
      sub={
        conflict ? (
          <span role="alert" id={statusId} {...stylex.props(sc.warn)}>
            {t("shortcuts.conflict", { action: conflict.with.label })}
          </span>
        ) : mine ? (
          <span role="status" id={statusId}>
            {t("shortcuts.recording")}
          </span>
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
              aria-describedby={statusId}
              value={displayedCombo ? formatForDisplay(normalizeHotkey(displayedCombo)) : ""}
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
  const { resetAll } = useShortcutOverrides.getState();
  const visible = useContentVisible();
  const filter = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [recording, setRecordingFor] = useState<Recording | null>(null);
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
    // Displayed keys render as separate caps, so spaced and compact spellings both match.
    const keys = q.replace(/\s+/g, "");
    return rows.filter(
      (row) =>
        row.label.toLowerCase().includes(q) ||
        (row.combo ?? "").toLowerCase().includes(q) ||
        (row.combo !== undefined &&
          formatForDisplay(normalizeHotkey(row.combo)).toLowerCase().includes(keys)),
    );
  }, [rows, query]);

  // An unapplied capture lives only while its recorder is visible and shown; otherwise
  // it would keep suppressing every global shortcut with no Cancel to reach.
  if (recording && !(visible && shown.some((row) => row.id === recording.commandId))) {
    setRecordingFor(null);
  }

  const recordingActive = recording !== null;
  return (
    <div {...stylex.props(sc.pane)}>
      <div {...stylex.props(sc.toolbar)}>
        <SearchField
          ref={filter}
          size="lg"
          disabled={recordingActive}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onClear={() => setQuery("")}
          clearLabel={t("common.clearSearch")}
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
