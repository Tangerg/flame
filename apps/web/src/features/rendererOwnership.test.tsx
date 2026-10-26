import { act, render, screen } from "@testing-library/react";
import { expect, it, onTestFinished } from "vitest";
import { createTestClient } from "@/test/renderClient";
import { useComposerText } from "@/features/composer/public/draft";
import { useActiveSessionId } from "@/features/agent/public/session";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";

function Draft({ label }: { label: string }) {
  const session = useActiveSessionId();
  const text = useComposerText();
  return <output aria-label={label}>{`${session}: ${text}`}</output>;
}

it("keeps concurrent renderer state and generation retirement with the injected instance", async () => {
  const first = createTestClient({ endpoint: "https://first.runtime.test" });
  const second = createTestClient({ endpoint: "https://second.runtime.test" });
  onTestFinished(() => first.dispose());
  onTestFinished(() => second.dispose());
  first.navigation.go({ session: "first-session" });
  second.navigation.go({ session: "second-session" });
  first.composer.access.replaceDraft({ text: "first draft" });
  second.composer.access.replaceDraft({ text: "second draft" });
  render(
    <>
      <first.Wrapper>
        <Draft label="First renderer" />
      </first.Wrapper>
      <second.Wrapper>
        <Draft label="Second renderer" />
      </second.Wrapper>
    </>,
  );
  expect(screen.getByLabelText("First renderer").textContent).toBe("first-session: first draft");
  expect(screen.getByLabelText("Second renderer").textContent).toBe("second-session: second draft");
  const firstCommand = first.agent.dependencies.commands.current();
  const secondCommand = second.agent.dependencies.commands.current();
  act(() => {
    first.projection.store.setState({
      connectionGeneration: RuntimeConnectionGeneration.forProcess("first-next"),
    });
    first.composer.access.replaceDraft({ text: "edited first draft" });
    first.directory.access.open();
  });
  expect(firstCommand.isCurrent()).toBe(false);
  expect(secondCommand.isCurrent()).toBe(true);
  expect(second.agent.dependencies.commands.current()).toBe(secondCommand);
  expect(second.directory.access.selection()).toBeNull();
  expect(screen.getByLabelText("Second renderer").textContent).toBe("second-session: second draft");
  await act(() => first.dispose());
  act(() => second.composer.access.replaceDraft({ text: "second still active" }));
  expect(screen.getByLabelText("Second renderer").textContent).toBe(
    "second-session: second still active",
  );
});
