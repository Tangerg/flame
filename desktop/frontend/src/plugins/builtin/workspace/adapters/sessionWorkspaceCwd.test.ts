import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { RpcError } from "@/rpc";
import { resolveActiveSessionWorkspaceCwd } from "./sessionWorkspaceCwd";

const { getActiveSessionId, getSession } = vi.hoisted(() => ({
  getActiveSessionId: vi.fn<() => string>(() => ""),
  getSession: vi.fn(),
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({
    client: () => ({ sessions: { get: getSession } }),
  }),
}));

afterEach(() => {
  getActiveSessionId.mockReturnValue("");
  getSession.mockReset();
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("active session workspace resolution", () => {
  it("resolves no active session to the runtime default workspace", async () => {
    await expect(
      resolveActiveSessionWorkspaceCwd({ getActiveSessionId }, new AbortController().signal),
    ).resolves.toEqual({ status: "resolved" });
    expect(getSession).not.toHaveBeenCalled();
  });

  it("uses the matching session-list projection", async () => {
    getActiveSessionId.mockReturnValue("ses_cached");
    queryClient.setQueryData(
      [AGENT_SESSIONS_KEY],
      [
        {
          id: "ses_cached",
          workspace: { path: "/cached/repo", availability: "available" },
        },
      ],
    );

    await expect(
      resolveActiveSessionWorkspaceCwd({ getActiveSessionId }, new AbortController().signal),
    ).resolves.toEqual({ status: "resolved", cwd: "/cached/repo" });
    expect(getSession).not.toHaveBeenCalled();
  });

  it("reads a draft session which is not present in the list projection", async () => {
    getActiveSessionId.mockReturnValue("ses_draft");
    queryClient.setQueryData([AGENT_SESSIONS_KEY], []);
    getSession.mockResolvedValue({ workspace: { ref: { path: "/draft/repo" } } });
    const signal = new AbortController().signal;

    await expect(resolveActiveSessionWorkspaceCwd({ getActiveSessionId }, signal)).resolves.toEqual(
      {
        status: "resolved",
        cwd: "/draft/repo",
      },
    );
    expect(getSession).toHaveBeenCalledWith("ses_draft", signal);
  });

  it("keeps a missing session unavailable until the projection changes", async () => {
    getActiveSessionId.mockReturnValue("ses_remote");
    getSession.mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "session missing",
        data: { type: "session_not_found" },
      }),
    );

    await expect(
      resolveActiveSessionWorkspaceCwd({ getActiveSessionId }, new AbortController().signal),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("preserves a transient read failure for the subscription retry owner", async () => {
    getActiveSessionId.mockReturnValue("ses_remote");
    getSession.mockRejectedValue(new Error("offline"));

    await expect(
      resolveActiveSessionWorkspaceCwd({ getActiveSessionId }, new AbortController().signal),
    ).rejects.toThrow("offline");
  });
});
