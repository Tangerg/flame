import * as stylex from "@stylexjs/stylex";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_FILE_RENDERER } from "@/plugins/sdk/kernelPoints";
import type { WorkspaceFileRendererProps } from "@/plugins/sdk/types/workspace";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { MarkdownMessage } from "./ui/markdown/MarkdownMessage";

const styles = stylex.create({ page: { paddingInline: space.s4, paddingBlock: space.s3 } });

function MarkdownFile({ content }: WorkspaceFileRendererProps) {
  return (
    <div data-quote-source="message" {...stylex.props(styles.page, typeStep.prose)}>
      <MarkdownMessage text={content} reveal="instant" />
    </div>
  );
}

export const markdownFile = definePlugin({
  name: "flame.builtin.markdown-file",
  setup(ctx) {
    for (const extension of ["md", "markdown", "mdx"]) {
      ctx.contribute(WORKSPACE_FILE_RENDERER, MarkdownFile, { key: extension });
    }
  },
});
