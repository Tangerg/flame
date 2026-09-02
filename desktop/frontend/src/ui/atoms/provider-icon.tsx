import { createElement, type ComponentType } from "react";
import Alibaba from "@lobehub/icons/es/Alibaba/components/Mono";
import Anthropic from "@lobehub/icons/es/Anthropic/components/Mono";
import Azure from "@lobehub/icons/es/Azure/components/Mono";
import DeepSeek from "@lobehub/icons/es/DeepSeek/components/Mono";
import Fireworks from "@lobehub/icons/es/Fireworks/components/Mono";
import Gemini from "@lobehub/icons/es/Gemini/components/Mono";
import Groq from "@lobehub/icons/es/Groq/components/Mono";
import HuggingFace from "@lobehub/icons/es/HuggingFace/components/Mono";
import Minimax from "@lobehub/icons/es/Minimax/components/Mono";
import Mistral from "@lobehub/icons/es/Mistral/components/Mono";
import Moonshot from "@lobehub/icons/es/Moonshot/components/Mono";
import Ollama from "@lobehub/icons/es/Ollama/components/Mono";
import OpenAI from "@lobehub/icons/es/OpenAI/components/Mono";
import OpenRouter from "@lobehub/icons/es/OpenRouter/components/Mono";
import Perplexity from "@lobehub/icons/es/Perplexity/components/Mono";
import Together from "@lobehub/icons/es/Together/components/Mono";
import XAI from "@lobehub/icons/es/XAI/components/Mono";
import Zhipu from "@lobehub/icons/es/Zhipu/components/Mono";
import type { IconSize } from "@/lib/iconScale";
import { Icon } from "@/ui/icons";

type BrandIcon = ComponentType<{ size?: number }>;

interface Brand {
  mark: BrandIcon;
  /** How the vendor writes it. `capitalize` gets "Openai" and "Deepseek", which is a
   *  different company's name than the one on the mark beside it. */
  name: string;
}

// Keyed by the Runtime's own provider id — the only string that reaches here. `alibaba`
// rather than `qwen` and `google` rather than `gemini` are ITS spellings, not the brand's;
// keying by the brand leaves the mark unreachable. Map, not object, because an object
// answers `constructor` with an inherited function that then renders as a component.
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
  ["ollama", { mark: Ollama, name: "Ollama" }],
  ["openai", { mark: OpenAI, name: "OpenAI" }],
  ["openrouter", { mark: OpenRouter, name: "OpenRouter" }],
  ["perplexity", { mark: Perplexity, name: "Perplexity" }],
  ["together", { mark: Together, name: "Together" }],
  ["xai", { mark: XAI, name: "xAI" }],
  ["zhipu", { mark: Zhipu, name: "Zhipu" }],
]);

/** The vendor's own spelling where this app knows it, else the id capitalised. */
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
        className="inline-grid shrink-0 place-items-center [&>svg]:size-full"
        style={{ width: `var(--icon-${size})`, height: `var(--icon-${size})` }}
      >
        {/* The mark is one of the module constants above, picked by name — never built
            here, which is why it is applied rather than written as `<Brand />`. */}
        {createElement(brand.mark, { size: 0 })}
      </span>
    );
  }
  return <Icon name="spark" size={size} />;
}
