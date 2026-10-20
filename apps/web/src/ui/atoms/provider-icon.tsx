import * as stylex from "@stylexjs/stylex";
import Alibaba from "@/ui/atoms/provider-marks/Alibaba.svg";
import Anthropic from "@/ui/atoms/provider-marks/Anthropic.svg";
import Azure from "@/ui/atoms/provider-marks/Azure.svg";
import DeepSeek from "@/ui/atoms/provider-marks/DeepSeek.svg";
import Fireworks from "@/ui/atoms/provider-marks/Fireworks.svg";
import Gemini from "@/ui/atoms/provider-marks/Gemini.svg";
import Groq from "@/ui/atoms/provider-marks/Groq.svg";
import HuggingFace from "@/ui/atoms/provider-marks/HuggingFace.svg";
import Minimax from "@/ui/atoms/provider-marks/Minimax.svg";
import Mistral from "@/ui/atoms/provider-marks/Mistral.svg";
import Moonshot from "@/ui/atoms/provider-marks/Moonshot.svg";
import OpenAI from "@/ui/atoms/provider-marks/OpenAI.svg";
import OpenRouter from "@/ui/atoms/provider-marks/OpenRouter.svg";
import Perplexity from "@/ui/atoms/provider-marks/Perplexity.svg";
import Together from "@/ui/atoms/provider-marks/Together.svg";
import XAI from "@/ui/atoms/provider-marks/XAI.svg";
import Zhipu from "@/ui/atoms/provider-marks/Zhipu.svg";
import { type ProductIconSize } from "@/lib/productIconScale";
import { Icon } from "@/ui/icons/index";
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

export function ProviderIcon({
  provider,
  size = "md",
}: {
  provider: string;
  size?: ProductIconSize;
}) {
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
