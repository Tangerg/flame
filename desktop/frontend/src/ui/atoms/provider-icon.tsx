import * as stylex from "@stylexjs/stylex";
import Alibaba from "./provider-marks/Alibaba.svg";
import Anthropic from "./provider-marks/Anthropic.svg";
import Azure from "./provider-marks/Azure.svg";
import DeepSeek from "./provider-marks/DeepSeek.svg";
import Fireworks from "./provider-marks/Fireworks.svg";
import Gemini from "./provider-marks/Gemini.svg";
import Groq from "./provider-marks/Groq.svg";
import HuggingFace from "./provider-marks/HuggingFace.svg";
import Minimax from "./provider-marks/Minimax.svg";
import Mistral from "./provider-marks/Mistral.svg";
import Moonshot from "./provider-marks/Moonshot.svg";
import OpenAI from "./provider-marks/OpenAI.svg";
import OpenRouter from "./provider-marks/OpenRouter.svg";
import Perplexity from "./provider-marks/Perplexity.svg";
import Together from "./provider-marks/Together.svg";
import XAI from "./provider-marks/XAI.svg";
import Zhipu from "./provider-marks/Zhipu.svg";
import type { IconSize } from "@/lib/iconScale";
import { Icon } from "@/ui/icons";

const styles = stylex.create({
  mark: {
    display: "inline-block",
    flexShrink: 0,
    backgroundColor: "currentColor",
    maskSize: "contain",
    maskRepeat: "no-repeat",
    maskPosition: "center",
  },
});

interface Brand {
  mark: string;
  name: string;
}

const BRAND = new Map<string, Brand>([
  ["alibaba", { mark: Alibaba, name: "Alibaba" }],
  ["anthropic", { mark: Anthropic, name: "Anthropic" }],
  ["azureopenai", { mark: Azure, name: "Azure OpenAI" }],
  ["deepseek", { mark: DeepSeek, name: "DeepSeek" }],
  ["fireworks", { mark: Fireworks, name: "Fireworks" }],
  ["google", { mark: Gemini, name: "Google" }],
  ["groq", { mark: Groq, name: "Groq" }],
  ["huggingface", { mark: HuggingFace, name: "Hugging Face" }],
  ["minimax", { mark: Minimax, name: "MiniMax" }],
  ["mistral", { mark: Mistral, name: "Mistral" }],
  ["moonshot", { mark: Moonshot, name: "Moonshot" }],
  ["openai", { mark: OpenAI, name: "OpenAI" }],
  ["openrouter", { mark: OpenRouter, name: "OpenRouter" }],
  ["perplexity", { mark: Perplexity, name: "Perplexity" }],
  ["together", { mark: Together, name: "Together" }],
  ["xai", { mark: XAI, name: "xAI" }],
  ["zhipu", { mark: Zhipu, name: "Zhipu" }],
]);

export function providerDisplayName(provider: string): string {
  return (
    BRAND.get(provider.toLowerCase())?.name ?? provider.charAt(0).toUpperCase() + provider.slice(1)
  );
}

export function ProviderIcon({ provider, size = "md" }: { provider: string; size?: IconSize }) {
  const brand = BRAND.get(provider.toLowerCase());
  if (brand) {
    return (
      <span
        aria-hidden
        data-slot="provider-mark"
        {...stylex.props(styles.mark)}
        style={{
          width: `var(--icon-${size})`,
          height: `var(--icon-${size})`,
          maskImage: `url("${brand.mark}")`,
        }}
      />
    );
  }
  return <Icon name="server" size={size} />;
}
