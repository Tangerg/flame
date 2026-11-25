import { z } from "zod";
import type { InvocationOrigin } from "@flame/backend-sdk/mcp";
import { FIELD_LIMITS, LIMITS } from "./limits";

// The component bundle cannot load the Node-only SDK, so an author's origin
// mirrors its vocabulary under a compile-time check.
const INVOCATION_ORIGINS: typeof InvocationOrigin = {
  Human: "human",
  Model: "model",
  Automatic: "automatic",
};
// Who recorded a review or a comment: the original invocation, not a name.
export const authorSchema = z.strictObject({
  origin: z.enum(INVOCATION_ORIGINS),
  invocationId: z.string(),
});

const hash = z.string().regex(/^[a-f0-9]{64}$/);
export const checkSchema = z.strictObject({
  code: z.string().min(1).max(100),
  severity: z.enum(["info", "warning", "error"]),
  message: z.string().min(1).max(4096),
  location: z.string().max(1024).optional(),
});
export const engineCatalogSchema = z.strictObject({
  platform: z.string(),
  configured: z.boolean(),
  configuration: z.record(z.string(), z.string()),
  limitations: z.array(z.string()),
});
export const renderReportSchema = z.strictObject({
  sourceRevision: hash,
  sourceSha256: hash,
  pdfSha256: hash,
  platform: z.string(),
  engines: z.array(z.strictObject({ name: z.string(), version: z.string() })),
  pages: z
    .array(
      z.strictObject({
        number: z.number().int().min(1),
        assetId: hash,
        sha256: hash,
      }),
    )
    .max(LIMITS.renderPages),
  checks: z.array(checkSchema).max(1000),
  extractedText: z.string().max(LIMITS.renderTextBytes),
  fontStatus: z.literal("unverified"),
});
export const reviewReportSchema = z.strictObject({
  sourceRevision: hash,
  sourceSha256: hash,
  renderId: hash,
  pages: z
    .array(z.number().int().min(1).max(LIMITS.renderPages))
    .min(1)
    .max(LIMITS.renderPages),
  verdict: z.enum(["accepted", "changes_requested"]),
  notes: z.string().min(1).max(FIELD_LIMITS.reviewNotes),
  reviewer: authorSchema,
});
export type Check = z.infer<typeof checkSchema>;
export type RenderReport = z.infer<typeof renderReportSchema>;
export type EngineCatalog = z.infer<typeof engineCatalogSchema>;

export interface EngineRenderReport {
  platform: string;
  engines: Array<{ name: string; version: string }>;
  pages: Array<{ number: number; filename: string }>;
  checks: Check[];
  extractedText: string;
  fontStatus: "unverified";
}
