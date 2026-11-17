import type { CodeSourceProps } from "@flame/ui";
import { PierreFile } from "./PierreFile";
export function HighlightedCodeSource({ source, language, wrapped }: CodeSourceProps) {
  return <PierreFile source={source} language={language} wrapped={wrapped} />;
}
