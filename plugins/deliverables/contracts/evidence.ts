import { z } from "zod";
import { LIMITS } from "./limits";

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
  notes: z.string().min(1).max(12000),
  reviewer: z.strictObject({
    origin: z.enum(["human", "model", "automatic"]),
    invocationId: z.string(),
  }),
});
export type Check = z.infer<typeof checkSchema>;
export type RenderReport = z.infer<typeof renderReportSchema>;
export type ReviewReport = z.infer<typeof reviewReportSchema>;
export type EngineCatalog = z.infer<typeof engineCatalogSchema>;

export interface EngineRenderReport {
  platform: string;
  engines: Array<{ name: string; version: string }>;
  pages: Array<{ number: number; filename: string }>;
  checks: Check[];
  extractedText: string;
  fontStatus: "unverified";
}
