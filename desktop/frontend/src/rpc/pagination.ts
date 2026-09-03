import { MAXIMUM_PAGINATION_CURSOR_CHARACTERS } from "@flame/runtime-contract/wire";

export interface CursorPage<T = unknown> {
  data: T[];
  nextCursor?: string;
}

type PageItem<P extends CursorPage> = P["data"][number];

export interface PaginationPolicy {
  readonly maximumPageRequests: number;
  readonly maximumRowsPerPage: number;
  readonly maximumCollectedRows: number;
  readonly maximumCursorCodeUnits: number;
  readonly maximumRetainedCursorCodeUnits: number;
}

/** Construct one finite traversal policy; zero is never a default/unbounded mode. */
export function createPaginationPolicy(policy: PaginationPolicy): Readonly<PaginationPolicy> {
  for (const [field, value] of Object.entries(policy)) {
    if (!Number.isSafeInteger(value) || value <= 0) {
      throw new TypeError(`pagination policy ${field} must be a positive safe integer`);
    }
  }
  if (policy.maximumRetainedCursorCodeUnits < policy.maximumCursorCodeUnits) {
    throw new TypeError(
      "pagination retained cursor capacity must hold at least one maximum-size cursor",
    );
  }
  return Object.freeze({ ...policy });
}

// One SDK traversal can cover 204,800 ordinary 100-row Runtime pages while
// remaining finite. Array collection is intentionally tighter than streaming;
// cursors are exact strings but their aggregate retained material is 1 MiB.
export const SDK_PAGINATION_POLICY = createPaginationPolicy({
  maximumPageRequests: 2_048,
  maximumRowsPerPage: 1_000,
  maximumCollectedRows: 100_000,
  // Runtime cursors are ASCII, so its wire-character contract is the exact
  // JavaScript code-unit contract too.
  maximumCursorCodeUnits: MAXIMUM_PAGINATION_CURSOR_CHARACTERS,
  maximumRetainedCursorCodeUnits: 1 * 1_024 * 1_024,
});

export const PaginationViolation = {
  cursorCycle: "cursorCycle",
  cursorTooLarge: "cursorTooLarge",
  cursorRetentionExceeded: "cursorRetentionExceeded",
  pageCapacityExceeded: "pageCapacityExceeded",
  pageRowsExceeded: "pageRowsExceeded",
  collectionCapacityExceeded: "collectionCapacityExceeded",
} as const;

export type PaginationViolation = (typeof PaginationViolation)[keyof typeof PaginationViolation];

/** Raised when a Runtime page cannot belong to one finite exact traversal. */
export class PaginationError extends Error {
  readonly violation: PaginationViolation;
  readonly cursor?: string;
  readonly limit: number;
  readonly observed: number;

  constructor(
    violation: PaginationViolation,
    detail: string,
    facts: { cursor?: string; limit: number; observed: number },
  ) {
    super(`pagination ${detail}`);
    this.name = "PaginationError";
    this.violation = violation;
    this.cursor = facts.cursor;
    this.limit = facts.limit;
    this.observed = facts.observed;
  }
}

/**
 * Still a real Promise: `await call` returns its FIRST wire page while the auto-paging
 * members walk the rest, starting from the cursor the original request supplied and
 * preserving every other request field on continuation calls.
 */
export interface AutoPagingPromise<P extends CursorPage>
  extends Promise<P>, AsyncIterable<PageItem<P>> {
  pages(): AsyncIterable<P>;
  autoPagingToArray(): Promise<PageItem<P>[]>;
  autoPagingEach(
    visitor: (item: PageItem<P>) => void | boolean | Promise<void | boolean>,
  ): Promise<void>;
}

/** Build the SDK behavior for one Registry-classified cursor method. */
export function createAutoPagingPromise<P extends CursorPage>(
  fetchPage: (cursor?: string) => Promise<P>,
  policy: Readonly<PaginationPolicy>,
  initialCursor?: string,
): AutoPagingPromise<P> {
  const validatedPolicy = createPaginationPolicy(policy);
  const initialCursorError = cursorSizeError(initialCursor, validatedPolicy);
  const firstPage = initialCursorError
    ? Promise.reject<P>(initialCursorError)
    : fetchPage(initialCursor);

  const pages = (): AsyncIterable<P> => ({
    async *[Symbol.asyncIterator]() {
      const seen = new Set<string>();
      let retainedCursorCodeUnits = 0;
      if (initialCursor) {
        seen.add(initialCursor);
        retainedCursorCodeUnits = initialCursor.length;
      }
      let pageRequests = 1;

      let page = await firstPage;
      for (;;) {
        if (page.data.length > validatedPolicy.maximumRowsPerPage) {
          throw new PaginationError(
            PaginationViolation.pageRowsExceeded,
            "page row capacity exceeded",
            {
              limit: validatedPolicy.maximumRowsPerPage,
              observed: page.data.length,
            },
          );
        }
        const cursor = page.nextCursor;
        if (cursor) {
          const sizeError = cursorSizeError(cursor, validatedPolicy);
          if (sizeError) throw sizeError;
          if (seen.has(cursor)) {
            throw new PaginationError(
              PaginationViolation.cursorCycle,
              `cursor did not advance: ${JSON.stringify(cursor)}`,
              { cursor, limit: seen.size, observed: seen.size + 1 },
            );
          }
          if (pageRequests >= validatedPolicy.maximumPageRequests) {
            throw new PaginationError(
              PaginationViolation.pageCapacityExceeded,
              "page request capacity exceeded",
              {
                cursor,
                limit: validatedPolicy.maximumPageRequests,
                observed: pageRequests + 1,
              },
            );
          }
          if (
            retainedCursorCodeUnits >
            validatedPolicy.maximumRetainedCursorCodeUnits - cursor.length
          ) {
            throw new PaginationError(
              PaginationViolation.cursorRetentionExceeded,
              "retained cursor capacity exceeded",
              {
                cursor,
                limit: validatedPolicy.maximumRetainedCursorCodeUnits,
                observed: retainedCursorCodeUnits + cursor.length,
              },
            );
          }
          seen.add(cursor);
          retainedCursorCodeUnits += cursor.length;
          pageRequests += 1;
        }
        yield page;
        if (!cursor) return;
        page = await fetchPage(cursor);
      }
    },
  });

  const items = async function* (): AsyncIterableIterator<PageItem<P>> {
    for await (const page of pages()) {
      yield* page.data as PageItem<P>[];
    }
  };

  const autoPagingToArray = async (): Promise<PageItem<P>[]> => {
    const result: PageItem<P>[] = [];
    for await (const page of pages()) {
      if (result.length > validatedPolicy.maximumCollectedRows - page.data.length) {
        throw new PaginationError(
          PaginationViolation.collectionCapacityExceeded,
          "collection row capacity exceeded",
          {
            limit: validatedPolicy.maximumCollectedRows,
            observed: result.length + page.data.length,
          },
        );
      }
      result.push(...(page.data as PageItem<P>[]));
    }
    return result;
  };

  const autoPagingEach = async (
    visitor: (item: PageItem<P>) => void | boolean | Promise<void | boolean>,
  ): Promise<void> => {
    for await (const item of items()) {
      if ((await visitor(item)) === false) return;
    }
  };

  return Object.assign(firstPage, {
    pages,
    autoPagingToArray,
    autoPagingEach,
    [Symbol.asyncIterator]: items,
  });
}

function cursorSizeError(
  cursor: string | undefined,
  policy: Readonly<PaginationPolicy>,
): PaginationError | undefined {
  if (cursor === undefined || cursor.length <= policy.maximumCursorCodeUnits) return undefined;
  return new PaginationError(PaginationViolation.cursorTooLarge, "cursor capacity exceeded", {
    cursor,
    limit: policy.maximumCursorCodeUnits,
    observed: cursor.length,
  });
}
