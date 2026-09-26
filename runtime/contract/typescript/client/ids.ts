declare const sessionIdBrand: unique symbol;
declare const runIdBrand: unique symbol;
declare const segmentIdBrand: unique symbol;
declare const itemIdBrand: unique symbol;

export type SessionId = string & { readonly [sessionIdBrand]: never };
export type RunId = string & { readonly [runIdBrand]: never };
export type SegmentId = string & { readonly [segmentIdBrand]: never };
export type ItemId = string & { readonly [itemIdBrand]: never };

export const asSessionId = (s: string): SessionId => s as SessionId;
export const asRunId = (s: string): RunId => s as RunId;
export const asSegmentId = (s: string): SegmentId => s as SegmentId;
export const asItemId = (s: string): ItemId => s as ItemId;
