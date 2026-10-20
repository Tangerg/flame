import { type CodeSourceProps } from "@flame/ui";
import { stripCodeWrapper, useHighlightedCode } from "@/lib/highlight/useCodeHighlight";
export function HighlightedCodeSource({ source, language }: CodeSourceProps) {
  const html = useHighlightedCode(source, language);
  if (html === null) return <code>{source}</code>;
  return <code dangerouslySetInnerHTML={{ __html: stripCodeWrapper(html, "") }} />;
}
