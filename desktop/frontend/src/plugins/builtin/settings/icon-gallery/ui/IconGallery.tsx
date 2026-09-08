import * as stylex from "@stylexjs/stylex";
import { useMemo, useState } from "react";
import { ScrollArea, SearchField } from "@/ui";
import { useT } from "@/lib/i18n";
import { IconMap, rawToc } from "./iconMap";
import { color, corner, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";
import { gallerySpread, galleryStyles as g } from "./galleryStyles";

// The three groups `@lobehub/icons` sorts its catalogue into.
type GroupKey = "model" | "provider" | "application";

const GROUP_TITLE_KEYS: Record<GroupKey, string> = {
  model: "iconGallery.group.model",
  provider: "iconGallery.group.provider",
  application: "iconGallery.group.application",
};

const ig = stylex.create({
  page: { display: "flex", height: "100%", minHeight: 0, flexDirection: "column" },
  masthead: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s4,
    paddingInline: space.s5,
    paddingBlock: space.s4,
  },
  title: { color: color.fg, fontWeight: weight.medium, fontSize: "var(--text-display-sm)" },
  sub: { marginTop: space.s1, color: color.fgMuted },
  search: { width: "calc(var(--spacing) * 60)" },
  section: {
    paddingInline: space.s5,
    paddingTop: "calc(var(--spacing) * 4.5)",
    paddingBottom: space.s3,
  },
  sectionPad: { paddingBottom: space.s2_5 },
  empty: {
    paddingInline: space.s5,
    paddingBlock: space.s16,
    textAlign: "center",
    color: color.fgFaint,
  },
  dot: {
    height: space.s2,
    width: space.s2,
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
  },
});

export function IconGallery() {
  const t = useT();
  const [query, setQuery] = useState("");

  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rawToc;
    return rawToc.filter(
      (e) => e.fullTitle.toLowerCase().includes(q) || e.id.toLowerCase().includes(q),
    );
  }, [query]);

  const grouped = useMemo(() => {
    const buckets: Record<GroupKey, typeof rawToc> = {
      model: [],
      provider: [],
      application: [],
    };
    for (const e of items) {
      if (e.group in buckets) buckets[e.group as GroupKey].push(e);
    }
    for (const k of Object.keys(buckets) as GroupKey[]) {
      buckets[k].sort((a, b) => a.fullTitle.localeCompare(b.fullTitle));
    }
    return buckets;
  }, [items]);

  return (
    <div {...stylex.props(ig.page)}>
      <div {...stylex.props(ig.masthead)}>
        <div>
          <div {...stylex.props(ig.title)}>@lobehub/icons</div>
          <div {...stylex.props(ig.sub, typeStep.uiMd)}>
            {t("iconGallery.subtitle", { count: rawToc.length })}
          </div>
        </div>
        <SearchField
          value={query}
          onValueChange={setQuery}
          aria-label={t("iconGallery.filterLabel")}
          placeholder={t("iconGallery.filterPlaceholder")}
          onClear={() => setQuery("")}
          clearLabel={t("iconGallery.clear")}
          className={stylex.props(ig.search).className}
        />
      </div>

      <ScrollArea>
        {(Object.keys(grouped) as GroupKey[]).map((key) => {
          const list = grouped[key];
          if (list.length === 0) return null;
          return (
            <section key={key} {...stylex.props(ig.section)}>
              <header {...stylex.props(g.sectionHead, ig.sectionPad, typeStep.uiSm)}>
                <span>{t(GROUP_TITLE_KEYS[key])}</span>
                <span {...stylex.props(g.count)}>{list.length}</span>
              </header>
              <div {...stylex.props(gallerySpread.large)}>
                {list.map((entry) => (
                  <IconCard key={entry.id} entry={entry} />
                ))}
              </div>
            </section>
          );
        })}
        {items.length === 0 && (
          <div {...stylex.props(ig.empty, typeStep.uiMd)}>
            {t("iconGallery.empty", { q: query })}
          </div>
        )}
      </ScrollArea>
    </div>
  );
}

function IconCard({ entry }: { entry: (typeof rawToc)[number] }) {
  const Component = IconMap[entry.id];
  return (
    <div title={`${entry.fullTitle} — ${entry.id}`} {...stylex.props(g.card, g.cardLarge)}>
      <div {...stylex.props(g.plate, g.plateLarge)}>
        {Component ? <Component size={28} /> : <span {...stylex.props(g.missing)}>?</span>}
      </div>
      <div {...stylex.props(g.name, typeStep.uiSm)}>{entry.fullTitle}</div>
      <div {...stylex.props(ss.lineTight, typeStep.uiXs)}>
        <span
          title={entry.color}
          className={stylex.props(ig.dot, corner.pill).className}
          style={{ background: entry.color }}
        />
        <code {...stylex.props(ss.mono, ss.muted, typeStep.uiXs)}>{entry.id}</code>
      </div>
    </div>
  );
}
