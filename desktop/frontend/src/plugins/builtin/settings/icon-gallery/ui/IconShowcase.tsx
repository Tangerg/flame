import * as stylex from "@stylexjs/stylex";
import { comboGlyph } from "@/lib/combo";
import { Trans, useT } from "@/lib/i18n";
import { IconMap, IconByName } from "./iconMap";
import { gap, SectionLabel, Tag, vocab } from "@/ui";
import { useCommandCombo } from "@/plugins/sdk";
import { COMMAND_MENU_COMMAND } from "@/plugins/builtin/command/command-menu/public/commandMenu";
import { color, leading, space, type as typeStep } from "@/styles/tokens.stylex";
import { gallerySpread, galleryStyles as g } from "./galleryStyles";

interface Section {
  titleKey: string;
  ids: string[];
}

const SECTIONS: Section[] = [
  {
    titleKey: "iconGallery.group.development",
    ids: [
      "terminal",
      "code",
      "file-diff",
      "git-fork",
      "network",
      "server",
      "workflow",
      "web-search",
    ],
  },
  {
    titleKey: "iconGallery.group.files",
    ids: [
      "file-text",
      "file-code",
      "file-output",
      "folder",
      "folder-search",
      "clipboard-list",
      "calendar-plus",
      "book-open",
    ],
  },
  {
    titleKey: "iconGallery.group.status",
    ids: ["target", "check", "alert", "question", "clock", "circle-dot", "pause", "stop"],
  },
  {
    titleKey: "iconGallery.group.security",
    ids: [
      "shield",
      "shield-check",
      "shield-alert",
      "lock",
      "lock-open",
      "key",
      "fingerprint",
      "shield-x",
    ],
  },
  {
    titleKey: "iconGallery.group.objects",
    ids: ["brain", "brain-off", "pin", "pin-off", "eye", "eye-off", "bell", "bell-off"],
  },
  {
    titleKey: "iconGallery.group.layout",
    ids: [
      "panel-left",
      "panel-right",
      "columns",
      "grid-2",
      "layers",
      "sliders-horizontal",
      "toggle-on",
      "toggle-off",
    ],
  },
];

const sh = stylex.create({
  page: { display: "flex", flexDirection: "column", gap: "calc(var(--spacing) * 4.5)" },
  intro: { marginBottom: space.s1, color: color.fgMuted, lineHeight: leading.body },
  em: { fontStyle: "normal", color: color.fg },
});

export function IconShowcase() {
  const t = useT();
  const combo = useCommandCombo(COMMAND_MENU_COMMAND);
  const total = Object.keys(IconByName).length;

  return (
    <div {...stylex.props(sh.page)}>
      <p {...stylex.props(sh.intro, typeStep.uiMd)}>
        <Trans
          i18nKey="iconGallery.showcase"
          values={{ count: total, pkg: "@flame/icons", combo: comboGlyph(combo ?? "") }}
          components={{
            code: <Tag size="md" ink="strong" />,
            em: <em className={stylex.props(sh.em).className} />,
          }}
        />
      </p>

      {SECTIONS.map((sec) => (
        <section key={sec.titleKey} {...stylex.props(vocab.column, gap.s2)}>
          <SectionLabel trailing={<span {...stylex.props(g.count)}>{sec.ids.length}</span>}>
            {t(sec.titleKey)}
          </SectionLabel>
          <div {...stylex.props(gallerySpread.small)}>
            {sec.ids.map((id) => (
              <ShowcaseCard key={id} id={id} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function ShowcaseCard({ id }: { id: string }) {
  const meta = IconByName[id]!;
  const Glyph = IconMap[meta.component]!;
  const title = meta.component;
  return (
    <div title={`${title} — ${id}`} {...stylex.props(g.card, g.cardSmall)}>
      <div {...stylex.props(g.plate, g.plateSmall)}>
        <Glyph size={24} />
      </div>
      <div {...stylex.props(g.name, typeStep.uiSm)}>{title}</div>
    </div>
  );
}
