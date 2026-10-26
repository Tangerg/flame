import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { beforeEach, describe, expect, it } from "vitest";

beforeEach(() => {
  testClient.navigation.go({ session: "", view: null, dock: null, settings: null });
  testClient.agent.sessions.store.setState({
    openSessionIds: [],
    lastSessionId: "",
  });
});

describe("Agent session state adapter continuity", () => {
  it("remembers the adjacent survivor after the active Session closes", () => {
    testClient.navigation.go({ session: "ses_a", view: "diff", dock: "terminal" });
    testClient.agent.sessions.store.setState({
      openSessionIds: ["ses_a", "ses_b"],
      lastSessionId: "ses_a",
    });

    testClient.agent.access.state.closeSession("ses_a");

    expect(testClient.navigation.get().session).toBe("ses_b");
    expect(testClient.navigation.get().view).toBeNull();
    expect(testClient.navigation.get().dock).toBe("terminal");
    expect(testClient.agent.sessions.store.getState().lastSessionId).toBe("ses_b");
  });

  it("retires a deleted deep-link's promoted view with the authoritative Session", () => {
    testClient.navigation.go({ session: "ses_gone", view: "timeline", dock: "diff" });
    testClient.agent.sessions.store.setState({
      openSessionIds: ["ses_live", "ses_gone"],
      lastSessionId: "ses_gone",
    });

    testClient.agent.access.state.reconcileSessions(["ses_live"]);

    expect(testClient.navigation.get()).toMatchObject({
      session: "ses_live",
      view: null,
      dock: "diff",
    });
    expect(testClient.agent.sessions.store.getState().lastSessionId).toBe("ses_live");
  });

  it("keeps the active Session's promoted view when a background Session closes", () => {
    testClient.navigation.go({ session: "ses_a", view: "diff", dock: "terminal" });
    testClient.agent.sessions.store.setState({
      openSessionIds: ["ses_a", "ses_b"],
      lastSessionId: "ses_a",
    });

    testClient.agent.access.state.closeSession("ses_b");

    expect(testClient.navigation.get()).toMatchObject({
      session: "ses_a",
      view: "diff",
      dock: "terminal",
    });
    expect(testClient.agent.sessions.store.getState().lastSessionId).toBe("ses_a");
  });

  it("keeps the active Session's promoted view when reconciliation only trims its peers", () => {
    testClient.navigation.go({ session: "ses_a", view: "timeline", dock: "diff" });
    testClient.agent.sessions.store.setState({
      openSessionIds: ["ses_a", "ses_gone"],
      lastSessionId: "ses_a",
    });

    testClient.agent.access.state.reconcileSessions(["ses_a"]);

    expect(testClient.navigation.get()).toMatchObject({
      session: "ses_a",
      view: "timeline",
      dock: "diff",
    });
    expect(testClient.agent.sessions.store.getState().lastSessionId).toBe("ses_a");
  });

  it("forgets a missing last Session when authoritative reconciliation leaves none", () => {
    testClient.navigation.go({ session: "ses_gone" });
    testClient.agent.sessions.store.setState({
      openSessionIds: ["ses_gone"],
      lastSessionId: "ses_gone",
    });

    testClient.agent.access.state.reconcileSessions([]);

    expect(testClient.navigation.get().session).toBe("");
    expect(testClient.agent.sessions.store.getState().lastSessionId).toBe("");
  });
});
