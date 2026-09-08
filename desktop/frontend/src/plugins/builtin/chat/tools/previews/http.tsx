import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import type { Tone } from "@/lib/tone";
import { Badge, Well } from "@/ui";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useT } from "@/lib/i18n";
import {
  projectFetchedPage,
  projectHttpPreview,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW } from "./previewChrome";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";
import { previewStyles as pv } from "./previewStyles";

const hp = stylex.create({
  head: { marginBottom: space.s1_5, display: "flex", alignItems: "center", gap: space.s2 },
  headPlain: { marginBottom: space.s1_5 },
  // Takes the row's spare width so the status and the duration stay at its two ends.
  spacer: { minWidth: space.s4, flex: 1 },
});

function statusTone(status: number): Tone | undefined {
  if (status >= 500) return "negative";
  if (status >= 400) return "warning";
  if (status >= 200 && status < 300) return "success";
  return undefined;
}

function HttpRequestPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const response = projectHttpPreview(tool.result);
  if (!response) {
    return (
      <div {...stylex.props(TEXT_PREVIEW)}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.requesting"
          idle="tools.preview.idle.noResponse"
        />
      </div>
    );
  }
  return (
    <div {...stylex.props(pv.inset)}>
      <div {...stylex.props(hp.head)}>
        <Badge tone={statusTone(response.status)} face="mono">
          {response.status}
        </Badge>
        {response.duration && (
          <span {...stylex.props(ct.mono, ct.faint, typeStep.uiXs)}>{response.duration}</span>
        )}
        {response.headers.length > 0 && (
          <span {...stylex.props(ct.faint, typeStep.uiSm)}>
            {t("tools.http.headers", { count: response.headers.length })}
          </span>
        )}
        <div {...stylex.props(hp.spacer)} />
        {response.truncated && <Badge>{t("tools.overflow.truncated")}</Badge>}
      </div>
      <ToolOutputPanel
        output={response.body}
        status={tool.status}
        idleLabel="tools.preview.idle.emptyBody"
      />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function WebFetchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const page = projectFetchedPage(tool.result);
  if (!page) {
    return (
      <div {...stylex.props(TEXT_PREVIEW)}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.fetching"
          idle="tools.preview.idle.noPage"
        />
      </div>
    );
  }
  return (
    <div {...stylex.props(pv.inset)}>
      <div {...stylex.props(hp.headPlain)}>
        <Badge face="mono">{page.format}</Badge>
      </div>
      <Well cap="md">{page.content}</Well>
      <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
    </div>
  );
}

export const httpPreviews = definePlugin({
  name: "flame.builtin.http-previews",
  setup(ctx) {
    for (const preview of toolPreviews({
      http_request: HttpRequestPreview,
      web_fetch: WebFetchPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
