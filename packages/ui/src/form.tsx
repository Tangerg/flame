import * as stylex from "@stylexjs/stylex";
import {
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  type AriaAttributes,
  type ComponentPropsWithRef,
  type ReactNode,
} from "react";
import { clsx } from "clsx";
import { CARD_SURFACE } from "./control-surface";
import { PanelLayoutContext } from "./panel-layout";
import { Metadata } from "./panel";
import { color, layout, type, weight } from "./tokens.stylex";

const styles = stylex.create({
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  field: {
    display: "grid",
    alignContent: "start",
    gap: layout.labelGap,
    minWidth: 0,
  },
  fieldBody: {
    display: "grid",
    alignContent: "start",
    gap: layout.labelGap,
    minWidth: 0,
  },
  fieldLabel: { color: color.fgSoft, fontWeight: weight.medium },
  form: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.sectionGap,
    minWidth: 0,
  },
  formActionGroup: { maxWidth: "100%" },
  formActions: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: layout.contentGap,
    minWidth: 0,
  },
  formActionsUnit: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.sectionGap,
    minWidth: 0,
  },
  formFields: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.fieldGap,
    minWidth: 0,
  },
  formMainActions: { marginInlineStart: "auto", justifyContent: "flex-end" },
  formRow: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,12rem),1fr))",
    alignItems: "start",
    gap: layout.fieldGap,
    minWidth: 0,
  },
  formSection: { margin: 0, padding: 0, borderWidth: 0, minWidth: 0 },
  legend: {
    padding: 0,
    marginBlockEnd: layout.contentGap,
    color: color.fgMuted,
    fontWeight: weight.semibold,
  },
  recordForm: {
    paddingInline: layout.cardPadding,
    paddingBlock: layout.cardPadding,
  },
});

export function FormRow({ children }: { children: ReactNode }) {
  return (
    <div data-slot="form-row" {...stylex.props(styles.formRow)}>
      {children}
    </div>
  );
}

export function FormSection({
  title,
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <fieldset {...stylex.props(styles.formSection)}>
      <legend {...stylex.props(styles.legend, type.uiSm)}>{title}</legend>
      <div {...stylex.props(styles.formFields)}>
        {description && <Metadata>{description}</Metadata>}
        {children}
      </div>
    </fieldset>
  );
}

export function FormActions({
  label,
  auxiliaryAction,
  feedback,
  children,
}: {
  label: string;
  auxiliaryAction?: ReactNode;
  feedback?: ReactNode;
  children: ReactNode;
}) {
  const groupRef = useRef<HTMLDivElement>(null);
  const panelLayout = useContext(PanelLayoutContext);
  useEffect(() => {
    const group = groupRef.current;
    if (!feedback || !panelLayout || !group) return;
    // Asynchronous results arrive without a focus event, so the retained
    // action focus is the only signal that the reader is still here.
    const focused = group.ownerDocument.activeElement;
    if (group.contains(focused)) panelLayout.keepBodyFocusVisible(focused);
  }, [feedback, panelLayout]);
  return (
    <div
      ref={groupRef}
      role="group"
      aria-label={label}
      data-focus-unit=""
      {...stylex.props(styles.formActionsUnit)}
    >
      <div {...stylex.props(styles.formActions)}>
        {auxiliaryAction && (
          <div {...stylex.props(styles.actions, styles.formActionGroup)}>
            {auxiliaryAction}
          </div>
        )}
        <div
          {...stylex.props(
            styles.actions,
            styles.formActionGroup,
            styles.formMainActions,
          )}
        >
          {children}
        </div>
      </div>
      {feedback}
    </div>
  );
}

export function Form({
  children,
  className,
  ...props
}: ComponentPropsWithRef<"form">) {
  const styled = stylex.props(styles.form);
  return (
    <form {...props} {...styled} className={clsx(styled.className, className)}>
      {children}
    </form>
  );
}

export function RecordForm({
  children,
  opener,
  ref,
  className,
  ...props
}: ComponentPropsWithRef<"section"> & {
  opener?: HTMLElement | (() => HTMLElement | null);
}) {
  const element = useRef<HTMLElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const form = element.current;
    if (!form || !opener) return;
    ownedFocus.current = false;
    return () => {
      ownedFocus.current = form.contains(form.ownerDocument.activeElement);
    };
  }, [opener]);
  useEffect(() => {
    const form = element.current;
    if (!form || !opener) return;
    return () => {
      // Passive cleanup runs after the opener's disabled state commits.
      if (
        !form.isConnected &&
        ownedFocus.current &&
        form.ownerDocument.activeElement === form.ownerDocument.body
      ) {
        const target = typeof opener === "function" ? opener() : opener;
        if (target?.isConnected) target.focus();
      }
    };
  }, [opener]);
  const styled = stylex.props(
    CARD_SURFACE.face,
    styles.form,
    styles.recordForm,
  );
  return (
    <section
      {...props}
      ref={(node) => {
        element.current = node;
        if (typeof ref === "function") return ref(node);
        if (ref) ref.current = node;
      }}
      {...styled}
      className={clsx(styled.className, className)}
    >
      {children}
    </section>
  );
}

export function FormField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label data-slot="form-field" {...stylex.props(styles.field)}>
      <span {...stylex.props(styles.fieldLabel, type.uiSm)}>{label}</span>
      <span {...stylex.props(styles.fieldBody)}>{children}</span>
    </label>
  );
}

export function FormGroup({
  label,
  children,
  ...props
}: {
  label: string;
  children: ReactNode;
} & Pick<AriaAttributes, "aria-invalid" | "aria-describedby">) {
  const labelId = useId();
  return (
    <div
      {...props}
      role="group"
      aria-labelledby={labelId}
      data-focus-unit=""
      data-slot="form-field"
      {...stylex.props(styles.field)}
    >
      <span id={labelId} {...stylex.props(styles.fieldLabel, type.uiSm)}>
        {label}
      </span>
      <div {...stylex.props(styles.fieldBody)}>{children}</div>
    </div>
  );
}
