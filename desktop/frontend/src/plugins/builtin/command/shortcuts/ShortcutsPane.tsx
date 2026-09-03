import { useMemo, useState } from "react";
import { Kbd, SearchField } from "@/ui";
import { useKeymap } from "@/plugins/host/keymap";
import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";

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
    <div className="flex flex-col gap-3">
      <SearchField
        size="lg"
        value={query}
        onValueChange={setQuery}
        placeholder={t("shortcuts.filter")}
        aria-label={t("shortcuts.filterAria")}
      />

      <div className="min-h-0 flex-1 overflow-auto rounded-lg border-[0.5px] border-field bg-transparent">
        {filtered.length === 0 ? (
          <div className="px-3 py-6 text-center text-ui-md text-fg-faint">
            {t("shortcuts.empty")}
          </div>
        ) : (
          <table className="w-full border-collapse text-left text-ui-md">
            <thead className="sticky top-0 bg-sunken text-ui-sm font-semibold text-fg-faint">
              <tr>
                <th className="px-3 py-1.5">{t("shortcuts.action")}</th>
                <th className="w-[160px] px-3 py-1.5 text-right">{t("shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => (
                <tr key={s.key} className="transition-colors hover:bg-hover">
                  <td className="px-3 py-1.5 text-fg">{s.label}</td>
                  <td className="px-3 py-1.5 text-right">
                    <span className="inline-flex items-center gap-1">
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
