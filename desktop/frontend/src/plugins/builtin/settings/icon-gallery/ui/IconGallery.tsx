import * as stylex from "@stylexjs/stylex";
import { useMemo, useState } from "react";
import { EmptyState, ScrollArea, SearchField, SectionLabel, vocab } from "@/ui";
import { AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { IconMap, icons } from "./iconMap";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { gallerySpread, galleryStyles as g } from "./galleryStyles";

const ig = stylex.create({
  masthead: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s4,
    paddingInline: space.s5,
    paddingBlock: space.s4,
  },
  title: { color: color.fg, fontWeight: weight.medium },
  sub: { marginTop: space.s1, color: color.fgMuted },
  search: { width: "calc(var(--spacing) * 60)" },
  section: {
    paddingInline: space.s5,
    paddingTop: "calc(var(--spacing) * 4.5)",
    paddingBottom: space.s3,
  },
  sectionPad: { paddingBottom: space.s2_5 },
});

export function IconGallery() {
  const t = useT();
  const [query, setQuery] = useState("");

  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return icons;
    return icons.filter((e) =>
      [e.name, e.component, e.label, ...e.tags].join(" ").toLowerCase().includes(q),
    );
  }, [query]);

  const grouped = useMemo(() => {
    const buckets = new Map<string, (typeof icons)[number][]>();
    for (const entry of items) {
      const bucket = buckets.get(entry.group) ?? [];
      bucket.push(entry);
      buckets.set(entry.group, bucket);
    }
    for (const bucket of buckets.values()) bucket.sort((a, b) => a.name.localeCompare(b.name));
    return buckets;
  }, [items]);

  return (
    <AgentWorkspaceView>
      <div {...stylex.props(ig.masthead)}>
        <div>
          <div {...stylex.props(ig.title, typeStep.displaySm)}>{t("iconGallery.title")}</div>
          <div {...stylex.props(ig.sub, typeStep.uiMd)}>
            {t("iconGallery.subtitle", { count: icons.length })}
          </div>
        </div>
        <SearchField
          value={query}
          onValueChange={setQuery}
          aria-label={t("iconGallery.filterLabel")}
          placeholder={t("iconGallery.filterPlaceholder")}
          onClear={() => setQuery("")}
          clearLabel={t("common.clear")}
          className={stylex.props(ig.search).className}
        />
      </div>

      <ScrollArea>
        {[...grouped]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([key, list]) => {
            if (list.length === 0) return null;
            return (
              <section key={key} {...stylex.props(ig.section)}>
                <SectionLabel
                  className={stylex.props(ig.sectionPad).className}
                  trailing={<span {...stylex.props(g.count)}>{list.length}</span>}
                >
                  {t(`iconGallery.group.${key.toLowerCase()}`)}
                </SectionLabel>
                <div {...stylex.props(gallerySpread.large)}>
                  {list.map((entry) => (
                    <IconCard key={entry.name} entry={entry} />
                  ))}
                </div>
              </section>
            );
          })}
        {items.length === 0 && (
          <EmptyState icon="search" title={t("iconGallery.empty", { q: query })} />
        )}
      </ScrollArea>
    </AgentWorkspaceView>
  );
}

function IconCard({ entry }: { entry: (typeof icons)[number] }) {
  const Component = IconMap[entry.component]!;
  return (
    <div title={`${entry.component} — ${entry.name}`} {...stylex.props(g.card, g.cardLarge)}>
      <div {...stylex.props(g.plate, g.plateLarge)}>
        <Component size={24} />
      </div>
      <div {...stylex.props(g.name, typeStep.uiSm)}>{entry.component}</div>
      <div {...stylex.props(vocab.lineTight, typeStep.uiXs)}>
        <code {...stylex.props(vocab.muted, typeStep.uiXs, face.mono)}>{entry.name}</code>
      </div>
    </div>
  );
}
