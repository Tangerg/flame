import { describe, expect, it, vi } from "vitest";
import {
  createAutoPagingPromise,
  createPaginationPolicy,
  PaginationError,
  PaginationViolation,
} from "./pagination";

const testPolicy = createPaginationPolicy({
  maximumPageRequests: 4,
  maximumRowsPerPage: 4,
  maximumCollectedRows: 8,
  maximumCursorCodeUnits: 8,
  maximumRetainedCursorCodeUnits: 16,
});

function pager(pages: Array<{ data: string[]; nextCursor?: string }>) {
  return vi.fn(async (cursor?: string) => {
    const index = cursor ? Number(cursor.slice(1)) : 0;
    return pages[index]!;
  });
}

describe("auto-paging promise", () => {
  it("remains awaitable as the first wire page", async () => {
    const fetchPage = pager([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);

    await expect(createAutoPagingPromise(fetchPage, testPolicy)).resolves.toEqual({
      data: ["a"],
      nextCursor: "p1",
    });
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it("iterates every row and preserves the original cursor", async () => {
    const fetchPage = pager([
      { data: ["unused"] },
      { data: ["b"], nextCursor: "p2" },
      { data: ["c", "d"] },
    ]);
    const call = createAutoPagingPromise(fetchPage, testPolicy, "p1");

    const rows: string[] = [];
    for await (const row of call) rows.push(row);

    expect(rows).toEqual(["b", "c", "d"]);
    expect(fetchPage.mock.calls).toEqual([["p1"], ["p2"]]);
  });

  it("exposes full pages for page-level side data", async () => {
    const fetchPage = pager([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);
    const call = createAutoPagingPromise(fetchPage, testPolicy);
    const pages = [];

    for await (const page of call.pages()) pages.push(page);

    expect(pages).toEqual([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);
  });

  it("collects rows and supports early visitor termination", async () => {
    const fetchPage = pager([{ data: ["a", "b"], nextCursor: "p1" }, { data: ["c"] }]);
    const call = createAutoPagingPromise(fetchPage, testPolicy);

    await expect(call.autoPagingToArray()).resolves.toEqual(["a", "b", "c"]);

    const visited: string[] = [];
    await call.autoPagingEach((row) => {
      visited.push(row);
      return row !== "b";
    });
    expect(visited).toEqual(["a", "b"]);
  });

  it("rejects a repeated continuation instead of truncating silently", async () => {
    const fetchPage = vi.fn(async () => ({ data: ["a"], nextCursor: "same" }));
    const call = createAutoPagingPromise(fetchPage, testPolicy);

    const error = await call.autoPagingToArray().catch((reason: unknown) => reason);
    expect(error).toBeInstanceOf(PaginationError);
    expect(error).toEqual(
      expect.objectContaining({
        violation: PaginationViolation.cursorCycle,
        cursor: "same",
      }),
    );
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });

  it("rejects invalid policies instead of treating zero as unbounded", () => {
    expect(() =>
      createPaginationPolicy({
        ...testPolicy,
        maximumPageRequests: 0,
      }),
    ).toThrow("must be a positive safe integer");
    expect(() =>
      createPaginationPolicy({
        ...testPolicy,
        maximumRetainedCursorCodeUnits: testPolicy.maximumCursorCodeUnits - 1,
      }),
    ).toThrow("must hold at least one maximum-size cursor");
  });

  it("rejects an oversized initial cursor before transport I/O", async () => {
    const fetchPage = pager([{ data: ["unused"] }]);
    const call = createAutoPagingPromise(fetchPage, testPolicy, "x".repeat(9));

    await expect(call).rejects.toMatchObject({ violation: PaginationViolation.cursorTooLarge });
    expect(fetchPage).not.toHaveBeenCalled();
  });

  it("rejects an oversized continuation before exposing that page", async () => {
    const fetchPage = pager([{ data: ["untrusted"], nextCursor: "x".repeat(9) }]);
    const visited: string[] = [];
    const call = createAutoPagingPromise(fetchPage, testPolicy);

    const error = await call
      .autoPagingEach((item) => {
        visited.push(item);
      })
      .catch((reason: unknown) => reason);
    expect(error).toMatchObject({ violation: PaginationViolation.cursorTooLarge });
    expect(visited).toEqual([]);
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it("rejects an infinite unique cursor chain at page capacity", async () => {
    const policy = createPaginationPolicy({
      ...testPolicy,
      maximumPageRequests: 2,
    });
    const fetchPage = vi.fn(async (cursor?: string) => ({
      data: [cursor ?? "first"],
      nextCursor: cursor === undefined ? "p1" : "p2",
    }));
    const call = createAutoPagingPromise(fetchPage, policy);

    await expect(call.autoPagingToArray()).rejects.toMatchObject({
      violation: PaginationViolation.pageCapacityExceeded,
      limit: 2,
      observed: 3,
    });
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });

  it("bounds retained cursor material, page rows, and array collection independently", async () => {
    const cursorPolicy = createPaginationPolicy({
      ...testPolicy,
      maximumCursorCodeUnits: 4,
      maximumRetainedCursorCodeUnits: 5,
    });
    const cursorFetch = vi.fn(async (cursor?: string) => ({
      data: ["row"],
      nextCursor: cursor === undefined ? "abc" : "def",
    }));
    await expect(
      createAutoPagingPromise(cursorFetch, cursorPolicy).autoPagingToArray(),
    ).rejects.toMatchObject({ violation: PaginationViolation.cursorRetentionExceeded });

    const rowPolicy = createPaginationPolicy({
      ...testPolicy,
      maximumRowsPerPage: 2,
    });
    await expect(
      createAutoPagingPromise(async () => ({ data: ["a", "b", "c"] }), rowPolicy)
        .pages()
        [Symbol.asyncIterator]()
        .next(),
    ).rejects.toMatchObject({ violation: PaginationViolation.pageRowsExceeded });

    const collectionPolicy = createPaginationPolicy({
      ...testPolicy,
      maximumCollectedRows: 2,
    });
    await expect(
      createAutoPagingPromise(
        pager([{ data: ["a", "b"], nextCursor: "p1" }, { data: ["c"] }]),
        collectionPolicy,
      ).autoPagingToArray(),
    ).rejects.toMatchObject({ violation: PaginationViolation.collectionCapacityExceeded });
  });
});
