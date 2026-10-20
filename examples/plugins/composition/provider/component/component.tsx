import type { PluginComponent } from "@flame/component-sdk";
import details from "../package/io.github.tangerg.flame/contracts/details.json";

export const Panel: PluginComponent<{ title: string }, { label: string }> = ({
  context,
  input,
  contribution,
}) => (
  <section aria-label={contribution?.label}>
    <h2>{contribution?.label}</h2>
    {context.compose({
      contract: JSON.stringify(details),
      target: {
        scope: "installation",
        reference: context.component.installationId,
      },
      attributes: {},
      input,
    })}
  </section>
);

export const Frame: PluginComponent<unknown, { label: string }> = ({
  contribution,
  children,
}) => (
  <section aria-label={contribution?.label}>
    <h2>{contribution?.label}</h2>
    {children}
  </section>
);

export const Summary: PluginComponent<unknown, { label: string }> = ({
  contribution,
}) => <p>{contribution?.label}</p>;
