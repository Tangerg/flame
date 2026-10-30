export const LIMITS = Object.freeze({
  maxRecords: 10_000,
  maxBytes: 512 * 1024 * 1024,
  maxResultBytes: 16 * 1024 * 1024,
  maxInputBytes: 256 * 1024,
  maxSurfaceBytes: 256 * 1024,
  maxApprovalBytes: 64 * 1024,
  minLeaseSeconds: 60,
  maxLeaseSeconds: 3_600,
  defaultLeaseSeconds: 900,
  maxHistoryPage: 50,
});
