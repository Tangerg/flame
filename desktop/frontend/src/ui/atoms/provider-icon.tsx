import { createElement, type ComponentType } from "react";
import Anthropic from "@lobehub/icons/es/Anthropic/components/Mono";
import DeepSeek from "@lobehub/icons/es/DeepSeek/components/Mono";
import Gemini from "@lobehub/icons/es/Gemini/components/Mono";
import Meta from "@lobehub/icons/es/Meta/components/Mono";
import Mistral from "@lobehub/icons/es/Mistral/components/Mono";
import Moonshot from "@lobehub/icons/es/Moonshot/components/Mono";
import Ollama from "@lobehub/icons/es/Ollama/components/Mono";
import OpenAI from "@lobehub/icons/es/OpenAI/components/Mono";
import Qwen from "@lobehub/icons/es/Qwen/components/Mono";
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

// Map, not object: keyed by a runtime-chosen name, and an object answers `constructor`
// with an inherited FUNCTION that then renders as a component.
//
// One table, two columns: the mark and the spelling are the same fact about the same brand,
// and a second table keyed the same way is a second thing to keep in step.
const BRAND = new Map<string, Brand>([
  ["deepseek", { mark: DeepSeek, name: "DeepSeek" }],
  ["openai", { mark: OpenAI, name: "OpenAI" }],
  ["anthropic", { mark: Anthropic, name: "Anthropic" }],
  ["claude", { mark: Anthropic, name: "Claude" }],
  ["gemini", { mark: Gemini, name: "Gemini" }],
  ["google", { mark: Gemini, name: "Google" }],
  ["meta", { mark: Meta, name: "Meta" }],
  ["llama", { mark: Meta, name: "Llama" }],
  ["mistral", { mark: Mistral, name: "Mistral" }],
  ["moonshot", { mark: Moonshot, name: "Moonshot" }],
  ["kimi", { mark: Moonshot, name: "Kimi" }],
  ["ollama", { mark: Ollama, name: "Ollama" }],
  ["qwen", { mark: Qwen, name: "Qwen" }],
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
