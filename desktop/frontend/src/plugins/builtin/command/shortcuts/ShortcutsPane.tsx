import * as stylex from "@stylexjs/stylex";
import { useMemo, useState } from "react";
import { Kbd, SearchField } from "@/ui";
import { useKeymap } from "@/plugins/host/keymap";
import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";
import { color, radius, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";

const sc = stylex.create({
  pane: { display: "flex", flexDirection: "column", gap: space.s3 },
  // A real table, because this IS tabular: an action and the keys that reach it.
  frame: {
    minHeight: 0,
    flex: 1,
    overflow: "auto",
    borderRadius: radius.lg,
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: "transparent",
  },
  empty: {
    paddingInline: space.s3,
    paddingBlock: space.s6,
    textAlign: "center",
    color: color.fgFaint,
  },
  table: { width: "100%", borderCollapse: "collapse", textAlign: "left" },
  head: {
    position: "sticky",
    top: 0,
    backgroundColor: surface.sunken,
    color: color.fgFaint,
    fontWeight: weight.semibold,
  },
  cell: { paddingInline: space.s3, paddingBlock: space.s1_5 },
  // A fixed measure: the key column must not widen because one shortcut has three chords.
  keyColumn: { width: "160px", textAlign: "right" },
  right: { textAlign: "right" },
  ink: { color: color.fg },
  row: {
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
  },
  keys: { display: "inline-flex", alignItems: "center", gap: space.s1 },
});

export function ShortcutsPane() {
  const t = useT();
  const shortcuts = useKeymap();
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const rows = shortcuts
      .filter((s) => s.description)
      .map((s) => ({ ...s, label: t(s.description ?? "") }))
      .sort((a, b) => a.label.localeCompare(b.label));
    if (!q) return rows;
    return rows.filter((s) => s.label.toLowerCase().includes(q) || s.key.toLowerCase().includes(q));
  }, [shortcuts, query, t]);

  return (
    <div {...stylex.props(sc.pane)}>
      <SearchField
        size="lg"
        value={query}
        onValueChange={setQuery}
        placeholder={t("shortcuts.filter")}
        aria-label={t("shortcuts.filterAria")}
      />

      <div {...stylex.props(sc.frame)}>
        {filtered.length === 0 ? (
          <div {...stylex.props(sc.empty, typeStep.uiMd)}>{t("shortcuts.empty")}</div>
        ) : (
          <table {...stylex.props(sc.table, typeStep.uiMd)}>
            <thead {...stylex.props(sc.head, typeStep.uiSm)}>
              <tr>
                <th {...stylex.props(sc.cell)}>{t("shortcuts.action")}</th>
                <th {...stylex.props(sc.cell, sc.keyColumn)}>{t("shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => (
                <tr key={s.key} {...stylex.props(sc.row)}>
                  <td {...stylex.props(sc.cell, sc.ink)}>{s.label}</td>
                  <td {...stylex.props(sc.cell, sc.right)}>
                    <span {...stylex.props(sc.keys)}>
                      {splitCombo(s.key).map((part, i) => (
                        <Kbd key={i}>{part}</Kbd>
                      ))}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
