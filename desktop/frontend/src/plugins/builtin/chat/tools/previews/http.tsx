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
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

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
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.requesting"
          idle="tools.preview.idle.noResponse"
        />
      </div>
    );
  }
  return (
    <div className="pt-1">
      <div className="mb-1.5 flex items-center gap-2">
        <Badge tone={statusTone(response.status)} className="font-mono">
          {response.status}
        </Badge>
        {response.duration && (
          <span className="font-mono text-ui-xs text-fg-faint">{response.duration}</span>
        )}
        {response.headers.length > 0 && (
          <span className="text-ui-sm text-fg-faint">
            {t("tools.http.headers", { count: response.headers.length })}
          </span>
        )}
        <div className="min-w-4 flex-1" />
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
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.fetching"
          idle="tools.preview.idle.noPage"
        />
      </div>
    );
  }
  return (
    <div className="pt-1">
      <div className="mb-1.5">
        <Badge className="font-mono">{page.format}</Badge>
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
