import { Fragment, memo } from "react";
import { useLayoutSlot } from "@/plugins/sdk";
import { PluginBoundary } from "./PluginBoundary";

interface Props {
  name: string;
  wrapper?: boolean;
  className?: string;
}

export const Slot = memo(function Slot({ name, wrapper, className }: Props) {
  const specs = useLayoutSlot(name);
  if (specs.length === 0) return null;

  const children = specs.map((spec) => {
    const Component = spec.component;
    const body = spec.className ? (
      <div className={spec.className}>
        <Component />
      </div>
    ) : (
      <Component />
    );
    return (
      <PluginBoundary key={spec.id} plugin={`layout:${name}:${spec.id}`} label={`${name} slot`}>
        {body}
      </PluginBoundary>
    );
  });

  if (wrapper || className) {
    return (
      <div data-slot={name} className={className}>
        {children}
      </div>
    );
  }
  return <Fragment>{children}</Fragment>;
});
