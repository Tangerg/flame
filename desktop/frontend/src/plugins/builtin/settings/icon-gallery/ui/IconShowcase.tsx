import * as stylex from "@stylexjs/stylex";
import { comboGlyph } from "@/lib/combo";
import { Trans, useT } from "@/lib/i18n";
import { IconMap, TocById } from "./iconMap";
import { gap, Tag, vocab } from "@/ui";
import { COMMAND, useExtensionByKey } from "@/plugins/sdk";
import { COMMAND_MENU_COMMAND } from "@/plugins/builtin/command/command-menu/public/commandMenu";
import { color, leading, space, type as typeStep } from "@/styles/tokens.stylex";
import { gallerySpread, galleryStyles as g } from "./galleryStyles";

interface Section {
  titleKey: string;
  ids: string[];
}

const SECTIONS: Section[] = [
  {
    titleKey: "iconGallery.section.frontierLabs",
    ids: [
      "OpenAI",
      "Anthropic",
      "Claude",
      "ClaudeCode",
      "Gemini",
      "Google",
      "Grok",
      "Meta",
      "DeepSeek",
      "Mistral",
      "Cohere",
      "Perplexity",
    ],
  },
  {
    titleKey: "iconGallery.section.cloudEnterprise",
    ids: [
      "Microsoft",
      "Azure",
      "Bedrock",
      "Aws",
      "GoogleCloud",
      "Nvidia",
      "IBM",
      "Apple",
      "Github",
    ],
  },
  {
    titleKey: "iconGallery.section.chineseEcosystem",
    ids: [
      "Qwen",
      "Doubao",
      "Kimi",
      "Wenxin",
      "Hunyuan",
      "ChatGLM",
      "Yi",
      "Minimax",
      "Spark",
      "SenseNova",
    ],
  },
  {
    titleKey: "iconGallery.section.localRuntimes",
    ids: [
      "Ollama",
      "LmStudio",
      "Vllm",
      "HuggingFace",
      "Together",
      "Groq",
      "Fireworks",
      "Replicate",
      "OpenRouter",
      "SiliconCloud",
    ],
  },
  {
    titleKey: "iconGallery.section.mediaGeneration",
    ids: [
      "Midjourney",
      "Stability",
      "Flux",
      "Runway",
      "Sora",
      "Kling",
      "Pika",
      "Suno",
      "ElevenLabs",
    ],
  },
  {
    titleKey: "iconGallery.section.devTools",
    ids: [
      "Cursor",
      "Windsurf",
      "Cline",
      "Codex",
      "Copilot",
      "GithubCopilot",
      "Trae",
      "RooCode",
      "LobeHub",
    ],
  },
];

const sh = stylex.create({
  page: { display: "flex", flexDirection: "column", gap: "calc(var(--spacing) * 4.5)" },
  intro: { marginBottom: space.s1, color: color.fgMuted, lineHeight: leading.body },
  // The prose marks a term rather than stressing it, so it takes the ink and not the slant.
  em: { fontStyle: "normal", color: color.fg },
});

export function IconShowcase() {
  const t = useT();
  const combo = useExtensionByKey(COMMAND, COMMAND_MENU_COMMAND)?.combo;
  const total = SECTIONS.reduce((n, s) => n + s.ids.length, 0);

  return (
    <div {...stylex.props(sh.page)}>
      <p {...stylex.props(sh.intro, typeStep.uiMd)}>
        <Trans
          i18nKey="iconGallery.showcase"
          values={{ count: total, pkg: "@lobehub/icons", combo: comboGlyph(combo ?? "") }}
          components={{
            code: <Tag size="md" ink="strong" />,
            em: <em className={stylex.props(sh.em).className} />,
          }}
        />
      </p>

      {SECTIONS.map((sec) => (
        <section key={sec.titleKey} {...stylex.props(vocab.column, gap.s2)}>
          <header {...stylex.props(g.sectionHead, typeStep.uiSm)}>
            <span>{t(sec.titleKey)}</span>
            <span {...stylex.props(g.count)}>{sec.ids.length}</span>
          </header>
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
  const Glyph = IconMap[id];
  const meta = TocById[id];
  const title = meta?.fullTitle ?? id;
  return (
    <div title={`${title} — ${id}`} {...stylex.props(g.card, g.cardSmall)}>
      <div {...stylex.props(g.plate, g.plateSmall)}>
        {Glyph ? <Glyph size={22} /> : <span {...stylex.props(g.missing)}>?</span>}
      </div>
      <div {...stylex.props(g.name, typeStep.uiSm)}>{title}</div>
    </div>
  );
}
