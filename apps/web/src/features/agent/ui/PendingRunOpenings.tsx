import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { prettyJSON } from "@qimu/sdk/json";
import * as stylex from "@stylexjs/stylex";
import { Button, Disclosure } from "@flame/ui";
import { color, face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";

import { useActiveSessionId } from "@/features/agent/application/session/activeSession";
import { type AgentOpeningCommand } from "@/features/agent/application/runtimeGateway";
const styles = stylex.create({
  card: {
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: space.s2,
    padding: space.s3,
    color: color.fg,
  },
  title: { fontWeight: weight.semibold },
  detail: { width: "100%", maxHeight: "40vh", overflow: "auto" },
  content: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  image: { maxWidth: "100%", maxHeight: "20vh" },
});

export function PendingRunOpenings() {
  const agentAccess = useAgentDependencies();

  const t = useT();
  const sessionId = useActiveSessionId();
  const commands = agentAccess.view.useOpeningCommands();
  return commands.map((command) => (
    <section
      key={command.idempotencyKey}
      {...stylex.props(styles.card, typeStep.uiMd)}
      aria-label={t(command.operation === "start" ? "agent.opening.start" : "agent.opening.resume")}
    >
      <div {...stylex.props(styles.title)}>
        {t(command.operation === "start" ? "agent.opening.start" : "agent.opening.resume")}
      </div>
      <div role="status">
        {t(command.receipt === "unknown" ? "agent.command.unknown" : "agent.opening.notRecorded")}
      </div>
      <div {...stylex.props(styles.detail)}>
        <Disclosure title={t("agent.opening.review")}>
          {command.input?.parts.map((block, index) =>
            block.kind === "text" ? (
              <p key={index} {...stylex.props(styles.content)}>
                {block.text}
              </p>
            ) : (
              <img
                key={index}
                {...stylex.props(styles.image)}
                src={`data:${block.mime};base64,${block.data}`}
                alt={t("agent.opening.review")}
              />
            ),
          )}
          <pre {...stylex.props(styles.content, face.mono, typeStep.uiSm)}>
            {prettyJSON(reviewedOptions(command))}
          </pre>
        </Disclosure>
      </div>
      <Button
        size="sm"
        onClick={() =>
          agentAccess.view.getSession(sessionId)?.retryOpening?.(command.idempotencyKey)
        }
      >
        {t("agent.command.retry")}
      </Button>
    </section>
  ));
}

function reviewedOptions(command: AgentOpeningCommand) {
  if (command.operation === "resume") return { responses: command.responses };
  return { ...command.options, generation: command.generation };
}
