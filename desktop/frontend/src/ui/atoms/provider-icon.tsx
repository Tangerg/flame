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

// A Map, not an object: the key is a provider name the runtime chose, and an object
// answers `constructor` and `toString` with inherited members — a provider named
// either would hand back a FUNCTION here and render it as a component.
const BRAND = new Map<string, BrandIcon>([
  ["deepseek", DeepSeek],
  ["openai", OpenAI],
  ["anthropic", Anthropic],
  ["claude", Anthropic],
  ["gemini", Gemini],
  ["google", Gemini],
  ["meta", Meta],
  ["llama", Meta],
  ["mistral", Mistral],
  ["moonshot", Moonshot],
  ["kimi", Moonshot],
  ["ollama", Ollama],
  ["qwen", Qwen],
  ["zhipu", Zhipu],
]);

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
        {createElement(brand, { size: 0 })}
      </span>
    );
  }
  return <Icon name="spark" size={size} />;
}
