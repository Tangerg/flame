import { useRef, useState, type ReactNode } from "react";
import type { Translate } from "@flame/component-sdk";
import { Button } from "@flame/ui";

// Authorization belongs to this mounted resource, never to authored local state.
// A changed source retires the grant, including when a later edit restores an old URL.
export function ExternalResource({
  source,
  scope,
  label,
  notice,
  inline = false,
  t,
  children,
}: {
  source: string;
  scope?: string;
  label: string;
  notice?: string;
  inline?: boolean;
  t: Translate;
  children: ReactNode;
}) {
  const identity = JSON.stringify([source, scope ?? null]);
  const [authorization, setAuthorization] = useState({
    identity,
    granted: false,
  });
  const resource = useRef<HTMLElement>(null);
  const granted = authorization.identity === identity && authorization.granted;
  if (authorization.identity !== identity)
    setAuthorization({ identity, granted: false });
  const Container = inline ? "span" : "div";
  return (
    <Container
      ref={(node) => {
        resource.current = node;
      }}
      tabIndex={-1}
      role="group"
      aria-label={label}
      style={{ minWidth: 0 }}
    >
      {granted ? (
        children
      ) : (
        <>
          <span style={{ display: "block" }}>
            {notice ?? t("resource.notice")}
          </span>
          <span style={{ display: "block", overflowWrap: "anywhere" }}>
            {source}
          </span>
          <Button
            onClick={(event) => {
              if (
                event.currentTarget ===
                event.currentTarget.ownerDocument.activeElement
              )
                resource.current?.focus({ preventScroll: true });
              setAuthorization({ identity, granted: true });
            }}
          >
            {t("resource.load", { resource: label })}
          </Button>
        </>
      )}
    </Container>
  );
}
