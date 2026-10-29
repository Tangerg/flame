import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { mount, useCurrentUse, type Mounted } from "@qimu/react";
import { type Use } from "@qimu/browser";
import {
  type ComponentContext,
  type PluginComponent,
  type SaveFileRequest,
} from "@flame/component-sdk";
import { type ComponentAcquisition } from "@/features/plugins/adapters/componentModules";
import * as stylex from "@stylexjs/stylex";

const styles = stylex.create({ container: { display: "contents" } });

type Props = ComponentProps<PluginComponent> & {
  acquire(parent?: Use): Promise<ComponentAcquisition>;
  wrap(children: ReactNode): ReactNode;
  report(error: unknown): void;
};
type ComponentInput = ComponentProps<PluginComponent>;
interface ContextProjection {
  source?: ComponentContext;
  value?: ComponentContext;
}

function ownedInput(
  input: ComponentInput,
  acquisition: ComponentAcquisition,
  projection: ContextProjection,
): ComponentInput {
  if (projection.source === input.context) return { ...input, context: projection.value! };
  const lifetime = acquisition.use.lifetime;
  const context: ComponentContext = Object.freeze({
    ...input.context,
    signal: AbortSignal.any([input.context.signal, lifetime.signal]),
    saveFile: (request: SaveFileRequest) => context.track(() => input.context.saveFile(request)),
    track: <P extends Promise<unknown>>(operation: (signal: AbortSignal) => P): P => {
      context.signal.throwIfAborted();
      let original!: P;
      let failure: { error: unknown } | undefined;
      // Lifetime registers the task before invoking this synchronous factory.
      // Return the original Promise so command identity and retry remain owned
      // by the Runtime client rather than copied onto a replacement operation.
      void lifetime.run(() => {
        try {
          original = operation(context.signal);
          return original;
        } catch (error) {
          failure = { error };
          throw error;
        }
      });
      if (failure) throw failure.error;
      return original;
    },
  });
  projection.source = input.context;
  projection.value = context;
  return { ...input, context };
}

export function NativeComponentMount({ acquire, wrap, report, ...input }: Props) {
  const parent = useCurrentUse();
  const container = useRef<HTMLDivElement>(null);
  const current = useRef<
    | {
        acquisition: ComponentAcquisition;
        mount: Mounted<ComponentInput>;
        projection: ContextProjection;
      }
    | undefined
  >(undefined);
  const latest = useRef(input);
  const [failure, setFailure] = useState<{ error: unknown }>();
  useLayoutEffect(() => {
    latest.current = input;
  });
  useEffect(() => {
    let retired = false;
    let owned: typeof current.current;
    const release = async () => {
      if (!owned) return;
      await owned.mount.close();
      await owned.acquisition.flush();
    };
    const setup = async () => {
      const acquisition = await acquire(parent);
      if (retired) {
        await acquisition.use.close();
        await acquisition.flush();
        return;
      }
      try {
        const projection: ContextProjection = {};
        const rendered = await mount(
          acquisition.use,
          acquisition.reference,
          acquisition.contract,
          container.current!,
          ownedInput(latest.current, acquisition, projection),
          (error) => {
            report(error);
            if (!retired) setFailure({ error });
          },
          wrap,
        );
        owned = { acquisition, mount: rendered, projection };
        if (retired) await release();
        else current.current = owned;
      } catch (error) {
        await acquisition.use.close();
        await acquisition.flush();
        throw error;
      }
    };
    const work = parent ? parent.lifetime.run(setup) : setup();
    void work.catch((error: unknown) => {
      report(error);
      if (!retired) setFailure({ error });
    });
    return () => {
      retired = true;
      if (current.current === owned) current.current = undefined;
      void release().catch(report);
    };
  }, [acquire, parent, report, wrap]);
  useLayoutEffect(() => {
    const owned = current.current;
    if (owned)
      void owned.mount.render(ownedInput(input, owned.acquisition, owned.projection)).catch(report);
  }, [input, report]);
  if (failure) throw failure.error;
  return <div ref={container} {...stylex.props(styles.container)} />;
}
