import { describe, expect, it } from "vitest";
import { commitApprovalArgs } from "@/features/chat/message/application/approvalArgsEditor";
import { stringifyJSON } from "@qimu/sdk/json";
describe("commitApprovalArgs", () => {
  it("preserves an exact edited value and distinguishes neighboring wide integers", () => {
    const original = '{"wide":9007199254740993,"decimal":1.0}';
    const changed = '{"wide":9007199254740994,"decimal":1.0}';
    expect(commitApprovalArgs(original, original)).toBeUndefined();
    expect(stringifyJSON(commitApprovalArgs(original, changed))).toBe(changed);
    expect(commitApprovalArgs("{}", "1.0")).toBeNull();
  });
  it("returns undefined when the JSON value is unchanged", () => {
    expect(
      commitApprovalArgs(
        '{"path":"/tmp","flags":["a","b"]}',
        '{ "flags": ["a", "b"], "path": "/tmp" }',
      ),
    ).toBeUndefined();
  });

  it("returns the edited args object when the value changed", () => {
    expect(commitApprovalArgs('{"path":"/tmp"}', '{"path":"/safe","force":false}')).toEqual({
      path: "/safe",
      force: false,
    });
  });

  it("accepts an empty edited object", () => {
    expect(commitApprovalArgs('{"path":"/tmp"}', "{}")).toEqual({});
  });

  it("rejects malformed or non-object JSON", () => {
    expect(commitApprovalArgs("{}", "{")).toBeNull();
    expect(commitApprovalArgs("{}", "[]")).toBeNull();
    expect(commitApprovalArgs("{}", '"text"')).toBeNull();
  });
});
