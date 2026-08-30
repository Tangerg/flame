// Phantom tags so a `RunId` cannot be passed where an `ItemId` is expected; both are plain
// strings at runtime.
//
// Business ids are ALWAYS server-generated (API.md §2.2) — the client mints only the
// JSON-RPC envelope id. The brand cannot be validated at runtime, so the `as<X>` helpers
// mark "checked at the parse site" and stay searchable.

declare const sessionIdBrand: unique symbol;
declare const runIdBrand: unique symbol;
declare const segmentIdBrand: unique symbol;
declare const itemIdBrand: unique symbol;

export type SessionId = string & { readonly [sessionIdBrand]: never };
export type RunId = string & { readonly [runIdBrand]: never };
// A single streamed segment of a Run (seg_…). A Run keeps ONE stable RunId
// across HITL resume; each resume opens a NEW segment (a fresh SegmentId).
export type SegmentId = string & { readonly [segmentIdBrand]: never };
export type ItemId = string & { readonly [itemIdBrand]: never };

export const asSessionId = (s: string): SessionId => s as SessionId;
export const asRunId = (s: string): RunId => s as RunId;
export const asSegmentId = (s: string): SegmentId => s as SegmentId;
export const asItemId = (s: string): ItemId => s as ItemId;
