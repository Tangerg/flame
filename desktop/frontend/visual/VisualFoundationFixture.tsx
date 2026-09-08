import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import {
  AgentAppShell,
  AgentComposerSurface,
  AgentContentCard,
  AgentRow,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";
import { Button, IconButton } from "@/ui";
import * as stylex from "@stylexjs/stylex";
import { fx } from "./fixtureStyles";
import { type as typeStep } from "@/styles/tokens.stylex";
import { cn } from "@/lib/classNames";

interface VisualFoundationFixtureProps {
  sidebarOpen: boolean;
}

const SESSION_ROWS = [
  { id: "api", title: "Refine runtime protocol", meta: "now", active: true },
  { id: "visual", title: "Polish desktop shell", meta: "12m", active: false },
  { id: "tests", title: "Close conformance gaps", meta: "1h", active: false },
] as const;

function WorkIndexFixture() {
  return (
    <div {...stylex.props(fx.pane)}>
      {/* No inset of its own. `pl-[78px]` sat here and never applied: `.agent-surface-header`
          owns `padding-inline` from the density vocabulary and is UNLAYERED, so it outranked a
          utility in `@layer utilities`. Migrating it to StyleX — also unlayered, and at a
          specificity nothing outranks — would have made a dead override live and moved the
          wordmark 78px. What the goldens have always shown is the header's own inset. */}
      <AgentSurfaceHeader divider={false}>
        <span {...stylex.props(fx.semibold, fx.ink, typeStep.uiMd)}>Flame</span>
        <span {...stylex.props(fx.minRail)} />
        <IconButton icon="search" size="sm" aria-label="Search" />
        <IconButton icon="edit" size="sm" aria-label="New session" />
      </AgentSurfaceHeader>
      <div className={cn("panel-scroll", stylex.props(fx.pane, fx.listPad).className)}>
        <div {...stylex.props(typeStep.uiXs, fx.specimenLabel)}>Work index</div>
        <AgentRow icon="folder" trailing={<span {...stylex.props(fx.figures)}>3</span>}>
          scope
        </AgentRow>
        <div {...stylex.props(fx.columnTight)}>
          {SESSION_ROWS.map((session) => (
            <AgentRow
              key={session.id}
              icon="chat"
              indent="nested"
              active={session.active}
              trailing={
                <span {...stylex.props(typeStep.uiXs, fx.mono, fx.faint, fx.figures)}>
                  {session.meta}
                </span>
              }
            >
              {session.title}
            </AgentRow>
          ))}
        </div>
      </div>
      <div {...stylex.props(fx.captionRow)}>
        <IconButton icon="settings" size="sm" aria-label="Settings" />
        <span {...stylex.props(fx.muted, typeStep.uiSm)}>Visual fixture</span>
      </div>
    </div>
  );
}

function ComposerFixture() {
  return (
    <AgentComposerSurface data-testid="composer" {...stylex.props(fx.relative)}>
      <div {...stylex.props(fx.editorBox, typeStep.uiMd)}>
        Ask Flame to inspect, change, or explain this workspace…
      </div>
      <div {...stylex.props(fx.footerBox)}>
        <Button variant="ghost" size="xs">
          Agent
        </Button>
        <Button variant="ghost" size="xs">
          Auto
        </Button>
        <span {...stylex.props(fx.minRail)} />
        <IconButton icon="arrow-up" size="md" aria-label="Send" variant="primary" />
      </div>
    </AgentComposerSurface>
  );
}

function FoundationSurface({ sidebarOpen }: { sidebarOpen: boolean }) {
  return (
    <AgentContentCard label="Visual foundation" data-testid="content-card">
      <AgentSurfaceHeader corner="window">
        <IconButton icon="panel-l" size="sm" aria-label="Toggle work index" />
        <span {...stylex.props(typeStep.uiSm, fx.mono, fx.faint)}>scope</span>
        <span {...stylex.props(fx.faint, typeStep.uiMd)}>/</span>
        <span {...stylex.props(fx.truncate, fx.semibold, fx.ink, typeStep.uiMd)}>
          Visual foundation
        </span>
        <AgentStatusPill tone="idle">Ready</AgentStatusPill>
        <span {...stylex.props(fx.minRail)} />
        <IconButton icon="panel-r" size="sm" aria-label="Open context dock" />
      </AgentSurfaceHeader>

      <div className={cn("panel-scroll", stylex.props(fx.pane).className)}>
        <div {...stylex.props(fx.measure)}>
          <div {...stylex.props(typeStep.uiXs, fx.specimenLabelFlush)}>
            Deterministic visual fixture
          </div>
          <h1 {...stylex.props(fx.heading, typeStep.displayLg)}>
            One visual language, one source of truth.
          </h1>
          {/* Two lines with room to spare on the second, on purpose. `text-wrap: pretty` — which
              `globals.css` gives every paragraph — optimises the last lines, and Chromium falls
              back to greedy wrapping under load. A caption sitting on the break boundary
              therefore wrapped one way when the suite ran alone and another when it ran with
              everything else, and the golden could not be photographed twice the same. One line
              is the only width at which no algorithm gets a vote. */}
          <p {...stylex.props(fx.lede, typeStep.uiMd)}>
            Production primitives, with viewport, locale and appearance held still.
          </p>

          <div {...stylex.props(fx.cardGrid)}>
            <section {...stylex.props(fx.card)}>
              <div {...stylex.props(fx.medium, fx.faint, typeStep.uiSm)}>TYPE LADDER</div>
              <div {...stylex.props(fx.rowBaseline)}>
                <span {...stylex.props(typeStep.ui2xs)}>9</span>
                <span {...stylex.props(typeStep.uiXs)}>10</span>
                <span {...stylex.props(typeStep.uiSm)}>11</span>
                <span {...stylex.props(typeStep.uiMd)}>12</span>
                <span {...stylex.props(typeStep.uiMd)}>13</span>
                <code {...stylex.props(typeStep.code, fx.mono)}>code 11</code>
              </div>
            </section>
            <section {...stylex.props(fx.card)}>
              <div {...stylex.props(fx.medium, fx.faint, typeStep.uiSm)}>SURFACE ROLES</div>
              <div {...stylex.props(fx.row)}>
                <Button variant="primary" size="sm">
                  Continue
                </Button>
                <Button variant="outline" size="sm">
                  Review
                </Button>
                <AgentStatusPill tone="waiting">Needs input</AgentStatusPill>
              </div>
            </section>
          </div>

          <div {...stylex.props(fx.minField)} />
          <ComposerFixture />
          <div {...stylex.props(fx.hairline)} />
        </div>
      </div>
      <output className="sr-only" data-testid="sidebar-state">
        {sidebarOpen ? "expanded" : "collapsed"}
      </output>
    </AgentContentCard>
  );
}

export function VisualFoundationFixture({ sidebarOpen }: VisualFoundationFixtureProps) {
  return (
    <AgentAppShell
      sidebarLabel="Work index"
      sidebarResizeLabel="Resize the work index"
      sidebarOpen={sidebarOpen}
      sidebarWidth={SIDEBAR_DEFAULT_WIDTH_PX}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the foundation fixture sidebar"
      sidebarCollapseLabel="Collapse the foundation fixture sidebar"
      sidebar={<WorkIndexFixture />}
      main={<FoundationSurface sidebarOpen={sidebarOpen} />}
    />
  );
}
