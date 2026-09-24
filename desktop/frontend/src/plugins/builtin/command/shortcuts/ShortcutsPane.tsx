import * as stylex from "@stylexjs/stylex";
import { useEffect, useMemo, useState } from "react";
import { Button, EmptyState, Kbd, SearchField, TextButton, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { comboFromEvent, normalizeCombo, splitCombo } from "@/lib/combo";
import {
  SHORTCUT,
  useEffectiveCommands,
  useExtensionPoint,
  useShortcutOverrides,
} from "@/plugins/sdk";
import { color, motion, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import { SettingsGroup } from "@/plugins/builtin/settings/kit";

const sc = stylex.create({
  pane: { display: "flex", flexDirection: "column", gap: space.s3 },
  toolbar: { display: "flex", alignItems: "center", gap: space.s2 },
  table: { width: "100%", borderCollapse: "collapse", textAlign: "left" },
  head: {
    backgroundColor: surface.sunken,
    color: color.fgFaint,
    fontWeight: weight.semibold,
  },
  cell: { paddingInline: space.s4, paddingBlock: space.s1_5 },
  keyColumn: { width: "260px", textAlign: "right" },
  right: { textAlign: "right" },
  row: {
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
  },
  keys: { display: "inline-flex", alignItems: "center", gap: space.s1 },
  controls: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s2,
  },
  note: { color: color.fgMuted },
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

export function ShortcutsPane() {
  const t = useT();
  const commands = useEffectiveCommands();
  const fixed = useExtensionPoint(SHORTCUT);
  const overrides = useShortcutOverrides((state) => state.overrides);
  const { rebind, reset, resetAll, setRecording } = useShortcutOverrides.getState();
  const [query, setQuery] = useState("");
  const [recording, setRecordingFor] = useState<Recording | null>(null);

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
    setRecording(recording !== null && recording.conflict === null);
    return () => setRecording(false);
  }, [recording, setRecording]);

  useEffect(() => {
    if (!recording || recording.conflict) return;
    const onKey = (event: KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (event.key === "Escape") {
        setRecordingFor(null);
        return;
      }
      const combo = comboFromEvent(event);
      if (!combo) return;
      const holder = rows.find(
        (row) =>
          row.id !== recording.commandId &&
          row.combo !== undefined &&
          normalizeCombo(row.combo) === combo,
      );
      if (holder) {
        setRecordingFor({ commandId: recording.commandId, conflict: { combo, with: holder } });
        return;
      }
      rebind(recording.commandId, combo);
      setRecordingFor(null);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [recording, rows, rebind]);

  return (
    <div {...stylex.props(sc.pane)}>
      <div {...stylex.props(sc.toolbar)}>
        <SearchField
          size="lg"
          value={query}
          onValueChange={setQuery}
          placeholder={t("shortcuts.filter")}
          aria-label={t("shortcuts.filterAria")}
        />
        {Object.keys(overrides).length > 0 && (
          <Button variant="ghost" onClick={resetAll}>
            {t("shortcuts.resetAll")}
          </Button>
        )}
      </div>

      <SettingsGroup>
        {shown.length === 0 ? (
          <EmptyState icon="search" title={t("shortcuts.empty")} />
        ) : (
          <table {...stylex.props(sc.table, typeStep.uiMd)}>
            <thead {...stylex.props(sc.head, typeStep.uiSm)}>
              <tr>
                <th {...stylex.props(sc.cell)}>{t("shortcuts.action")}</th>
                <th {...stylex.props(sc.cell, sc.keyColumn)}>{t("shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((row) => {
                const mine = recording?.commandId === row.id ? recording : null;
                return (
                  <tr key={row.id} {...stylex.props(sc.row)}>
                    <td {...stylex.props(sc.cell, vocab.ink)}>{row.label}</td>
                    <td {...stylex.props(sc.cell, sc.right)}>
                      {mine?.conflict ? (
                        <span role="alert" {...stylex.props(sc.controls, typeStep.uiSm)}>
                          <span {...stylex.props(sc.warn)}>
                            {t("shortcuts.conflict", { action: mine.conflict.with.label })}
                          </span>
                          {mine.conflict.with.editable ? (
                            <TextButton
                              tone="accent"
                              onClick={() => {
                                rebind(mine.conflict.with.id, null);
                                rebind(row.id, mine.conflict.combo);
                                setRecordingFor(null);
                              }}
                            >
                              {t("shortcuts.replace")}
                            </TextButton>
                          ) : null}
                          <TextButton tone="muted" onClick={() => setRecordingFor(null)}>
                            {t("common.cancel")}
                          </TextButton>
                        </span>
                      ) : mine ? (
                        <span role="status" {...stylex.props(sc.controls, sc.note, typeStep.uiSm)}>
                          {t("shortcuts.recording")}
                        </span>
                      ) : (
                        <span {...stylex.props(sc.controls)}>
                          {row.combo ? (
                            <Keys combo={row.combo} />
                          ) : (
                            <span {...stylex.props(sc.note, typeStep.uiSm)}>
                              {t("shortcuts.unbound")}
                            </span>
                          )}
                          {row.editable && (
                            <TextButton
                              tone="muted"
                              size="sm"
                              aria-label={t("shortcuts.editAria", { action: row.label })}
                              onClick={() => setRecordingFor({ commandId: row.id, conflict: null })}
                            >
                              {t("shortcuts.edit")}
                            </TextButton>
                          )}
                          {row.overridden && (
                            <TextButton tone="muted" size="sm" onClick={() => reset(row.id)}>
                              {t("shortcuts.reset")}
                            </TextButton>
                          )}
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </SettingsGroup>
    </div>
  );
}
