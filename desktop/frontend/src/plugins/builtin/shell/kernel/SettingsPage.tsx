import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { Suspense, useState } from "react";
import { Button, Icon, knownIconName, SearchField, SkeletonList, VerticalTabs, vocab } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import {
  openWorkspaceSettingsPane,
  selectWorkspaceChat,
  useWorkspaceSettingsPaneTarget,
} from "@/plugins/builtin/workspace/public/navigation";
import { useSettingsPanes } from "@/plugins/sdk";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const sp = stylex.create({
  title: { color: color.fg, fontWeight: weight.semibold },
  // A reading measure for the blurb: 60 characters, which the pane's width does not decide.
  //
  // No `margin: 0` beside the `marginTop`. They are two KEYS, so StyleX emits a class for each
  // and cannot merge them the way it merges two styles naming one key — the blurb's top margin
  // was whichever rule the bundler wrote second. The reset already zeroes every margin, so the
  // shorthand was saying nothing that needed saying and outranking something that did.
  blurb: {
    marginTop: space.s1_5,
    maxWidth: "60ch",
    lineHeight: "1.5rem",
    color: color.fgMuted,
  },
  body: { marginTop: space.s6, paddingBottom: space.s12 },
  backRow: { paddingInline: space.s4, paddingBottom: space.s4 },
  back: { marginBottom: space.s3, alignSelf: "flex-start" },
  // The back arrow leads rather than accompanies, so it opts out of the glyph step.
  backGlyph: { opacity: 1 },
});

const GROUPS: { id: string; labelKey: string }[] = [
  { id: "general", labelKey: "settings.group.general" },
  { id: "models", labelKey: "settings.group.models" },
  { id: "agent", labelKey: "settings.group.agent" },
  { id: "integrations", labelKey: "settings.group.integrations" },
  { id: "advanced", labelKey: "settings.group.advanced" },
];
const FALLBACK_GROUP = "advanced";

export function SettingsPage() {
  const t = useT();
  const panes = useSettingsPanes();
  const targetPane = useWorkspaceSettingsPaneTarget();
  const [query, setQuery] = useState("");
  const normalizedQuery = query.trim().toLocaleLowerCase();

  const known = new Set(GROUPS.map((g) => g.id));
  const grouped = GROUPS.map((g) => ({
    ...g,
    label: t(g.labelKey),
    items: panes
      .filter((p) => (p.group && known.has(p.group) ? p.group : FALLBACK_GROUP) === g.id)
      .map((p) => ({
        id: p.id,
        label: t(p.label),
        icon: knownIconName(p.icon),
        content: (
          <SettingsPaneFrame
            title={t(p.label)}
            description={p.description ? t(p.description) : undefined}
          >
            <PluginBoundary plugin={`settings:${p.id}`}>
              <Suspense fallback={<SkeletonList count={4} label={t("common.loading")} />}>
                <p.component />
              </Suspense>
            </PluginBoundary>
          </SettingsPaneFrame>
        ),
      }))
      .filter((item) =>
        normalizedQuery ? String(item.label).toLocaleLowerCase().includes(normalizedQuery) : true,
      ),
  })).filter((g) => g.items.length > 0);
  const visibleItems = grouped.flatMap((group) => group.items);
  const activeId =
    targetPane && visibleItems.some((p) => p.id === targetPane) ? targetPane : visibleItems[0]?.id;

  return (
    <VerticalTabs
      ariaLabel={t("settings.title")}
      groups={grouped}
      value={activeId}
      onValueChange={(pane) => {
        if (pane) openWorkspaceSettingsPane(pane);
      }}
      railHeader={
        <SettingsRailHeader
          query={query}
          onQueryChange={setQuery}
          searchPlaceholder={t("settings.searchPlaceholder")}
        />
      }
    />
  );
}

function SettingsPaneFrame({
  title,
  description,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section>
      <header>
        <h1 {...stylex.props(sp.title, typeStep.displayMd)}>{title}</h1>
        {description && <p {...stylex.props(sp.blurb, typeStep.uiMd)}>{description}</p>}
      </header>
      <div {...stylex.props(sp.body)}>{children}</div>
    </section>
  );
}

function SettingsRailHeader({
  query,
  onQueryChange,
  searchPlaceholder,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  searchPlaceholder: string;
}) {
  const t = useT();
  return (
    <div {...stylex.props(vocab.column)}>
      <AgentSurfaceHeader divider={false} corner="window" aria-hidden />
      <div {...stylex.props(sp.backRow)}>
        <Button
          type="button"
          variant="ghost"
          size="md"
          press="none"
          data-chrome-focus=""
          onClick={selectWorkspaceChat}
          className={stylex.props(sp.back).className}
        >
          <Icon name="arrow-left" size="md" className={stylex.props(sp.backGlyph).className} />
          <span>{t("settings.backToApp")}</span>
        </Button>
        <SearchField
          size="lg"
          value={query}
          onValueChange={onQueryChange}
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
        />
      </div>
    </div>
  );
}
