import * as stylex from "@stylexjs/stylex";
import { DataView } from "@/ui/atoms/data-view";
import { FilePath } from "@/ui/atoms/file-path";
import { SystemMessage } from "@/ui/atoms/system-message";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import { businessVariant, dataVariants, messageVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const CAPTURED_FILES = ["apps/web/src/App.tsx", "packages/ui/src/button.tsx", "README.md"];
const CAPTURED_FAILURE = new Error("The isolated fixture could not read its sample files");

export function DataExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(Object.values(dataVariants), variant);
  return (
    <div
      role="group"
      tabIndex={-1}
      aria-label={localize(messages.stories.data.title, locale)}
      {...stylex.props(styles.stack)}
    >
      <DataView
        items={
          scenario.id === dataVariants.ready.id
            ? CAPTURED_FILES
            : scenario.id === dataVariants.empty.id
              ? []
              : undefined
        }
        isLoading={scenario.id === dataVariants.loading.id}
        failure={scenario.id === dataVariants.failure.id ? CAPTURED_FAILURE : undefined}
        loadingLabel={localize(messages.variants.loading, locale)}
        empty={{
          icon: "folder",
          title: localize(messages.samples.emptyTitle, locale),
          sub: localize(messages.samples.emptyDescription, locale),
          size: "compact",
        }}
        error={{
          title: localize(messages.samples.errorTitle, locale),
          sub: localize(messages.samples.errorDescription, locale),
          size: "compact",
        }}
        onRetry={() => observe("data.retry")}
      >
        {(items) => (
          <div {...stylex.props(styles.rows)}>
            {items.map((path) => (
              <FilePath key={path} path={path} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}

export function MessageExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(messageVariants, variant);
  return (
    <SystemMessage
      variant={scenario.id}
      action={{
        label: localize(messages.samples.inspect, locale),
        onClick: () => observe("message.inspect", scenario.id),
      }}
    >
      {localize(messages.samples[scenario.id], locale)}
    </SystemMessage>
  );
}
