// One field list keeps SQLite metadata and isolated evidence accounting aligned.
const columns = {
  id: "id",
  kind: "kind",
  method: "method",
  input: "input",
  inputDigest: "digest",
  invocation: "invocation",
  admittedAt: "admitted_at",
} as const;

export type ReceiptEvidence = {
  readonly [field in keyof typeof columns]: string;
};

const encoder = new TextEncoder();

function size(value: string): number {
  return encoder.encode(value).byteLength;
}

export function evidenceBytes(evidence: ReceiptEvidence): number {
  return (Object.keys(columns) as (keyof typeof columns)[]).reduce(
    (total, field) => total + size(evidence[field]),
    0,
  );
}

// An unsettled outcome reserves the complete result bound; its parameter is
// maxResultBytes. SQLite reads octet lengths from record metadata.
export const receiptUsageSQL = `count(*) AS count, coalesce(sum(CASE WHEN outcome IS NULL THEN 1 ELSE 0 END),0) AS pending, coalesce(sum(${Object.values(
  columns,
)
  .map((column) => `octet_length(${column})`)
  .join(
    "+",
  )}+CASE WHEN outcome IS NULL THEN ? ELSE octet_length(outcome) END),0) AS bytes`;

export function receiptCapacity(
  records: Iterable<ReceiptEvidence & { readonly outcome?: string }>,
  limits: {
    readonly maxRecords: number;
    readonly maxBytes: number;
    readonly maxResultBytes: number;
  },
) {
  let count = 0;
  let pending = 0;
  let bytes = 0;
  for (const record of records) {
    count += 1;
    if (record.outcome === undefined) pending += 1;
    bytes +=
      evidenceBytes(record) +
      (record.outcome === undefined
        ? limits.maxResultBytes
        : size(record.outcome));
  }
  return {
    records: count,
    bytes,
    pending,
    maxRecords: limits.maxRecords,
    maxBytes: limits.maxBytes,
  };
}
