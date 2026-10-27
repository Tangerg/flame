import type { AcceptedDocument } from "../../../plugins/genui/contracts/document";

export const GENUI_REPORT: AcceptedDocument = {
  title: "Agent plugin delivery review",
  state: [
    { id: "amount", value: "4" },
    { id: "team", value: "Runtime team" },
    { id: "track", value: "balanced" },
    { id: "capabilities", value: ["genui"] },
    { id: "pilot", value: "internal" },
    { id: "approved", value: false },
    { id: "projection", value: true },
    { id: "budget", value: 50 },
    { id: "deadline", value: "2026-11-10" },
    { id: "notes", value: "Preserve the original execution owner." },
    { id: "submitted", value: "No scenario has been applied." },
    { id: "search", value: "" },
    { id: "sort", value: "" },
    { id: "page", value: 0 },
    { id: "selected", value: [] },
    { id: "tab", value: "delivery" },
    { id: "details", value: false },
    { id: "accordion", value: [] },
    { id: "carousel", value: 0 },
    { id: "popover", value: false },
    { id: "timer", value: 120 },
    { id: "timerRunning", value: false },
    { id: "flipped", value: false },
  ],
  actions: [
    {
      id: "applyScenario",
      steps: [
        {
          type: "set",
          state: "submitted",
          value: {
            call: "concat",
            args: [
              {
                array: [
                  { literal: "Applied scenario for " },
                  { state: "team" },
                  { literal: ": " },
                  { state: "amount" },
                  { literal: " delivery weeks." },
                ],
              },
            ],
          },
        },
      ],
    },
  ],
  operations: [],
  bindings: [],
  nodes: [
    {
      id: "introduction",
      type: "Text",
      props: {
        text: "Review a captured delivery plan, explore a local scenario, and inspect the original evidence.",
      },
    },
    {
      id: "metrics",
      type: "Grid",
      props: { columns: 2 },
      children: [
        {
          id: "audit",
          type: "Metric",
          props: {
            label: "Audit record",
            value: "9007199254740993",
            detail: "The exact identifier remains a string.",
          },
        },
        {
          id: "forecast",
          type: "Metric",
          props: {
            label: "Projected checks",
            value: {
              call: "multiply",
              args: [
                {
                  call: "coalesce",
                  args: [
                    { array: [{ call: "number", args: [{ state: "amount" }] }, { literal: 0 }] },
                  ],
                },
                { literal: 12 },
              ],
            },
            detail: "Twelve checks per delivery week.",
          },
        },
      ],
    },
    {
      id: "scenario",
      type: "Form",
      props: {
        title: "Delivery scenario",
        action: "applyScenario",
        submitLabel: "Apply scenario",
        resetLabel: "Reset scenario",
      },
      children: [
        {
          id: "scenarioFields",
          type: "Grid",
          props: { columns: 2 },
          children: [
            {
              id: "teamField",
              type: "Input",
              props: {
                field: "team",
                label: "Delivery team",
                rules: { required: true, maxLength: 80 },
              },
            },
            {
              id: "amountField",
              type: "Input",
              props: {
                field: "amount",
                label: "Delivery weeks",
                type: "number",
                rules: { required: true, minimum: 1, maximum: 24 },
              },
            },
            {
              id: "trackField",
              type: "Select",
              props: {
                field: "track",
                label: "Delivery track",
                options: [
                  { value: "balanced", label: "Balanced" },
                  { value: "focused", label: "Focused" },
                ],
              },
            },
            {
              id: "dateField",
              type: "DatePicker",
              props: {
                field: "deadline",
                label: "Review date",
                min: "2026-10-27",
                max: "2027-12-31",
              },
            },
          ],
        },
        {
          id: "advanced",
          type: "Disclosure",
          props: { field: "details", title: "Advanced scenario controls" },
          children: [
            {
              id: "capabilityField",
              type: "MultiSelect",
              props: {
                field: "capabilities",
                label: "Capabilities",
                options: [
                  { value: "genui", label: "Generative UI" },
                  { value: "computer", label: "Computer use" },
                  { value: "code", label: "Code mode" },
                ],
              },
            },
            {
              id: "pilotField",
              type: "RadioGroup",
              props: {
                field: "pilot",
                label: "Pilot audience",
                options: [
                  { value: "internal", label: "Internal" },
                  { value: "public", label: "Public" },
                ],
              },
            },
            {
              id: "budgetField",
              type: "Slider",
              props: { field: "budget", label: "Capacity percentage", min: 0, max: 100, step: 5 },
            },
            {
              id: "notesField",
              type: "TextArea",
              props: { field: "notes", label: "Review notes", rows: 3 },
            },
            {
              id: "approvalField",
              type: "Checkbox",
              props: { field: "approved", label: "Ready for a review" },
            },
          ],
        },
        {
          id: "projectionField",
          type: "Switch",
          props: { field: "projection", label: "Show the local projection" },
        },
      ],
    },
    {
      id: "scenarioStatus",
      type: "Callout",
      props: { title: "Local scenario", text: { state: "submitted" }, tone: "info" },
    },
    {
      id: "projectionNote",
      type: "If",
      props: { when: { state: "projection" } },
      children: [
        {
          id: "projectionText",
          type: "Text",
          props: {
            text: "Scenario changes are local to this view. The captured review remains available in the inspector.",
          },
        },
      ],
    },
    {
      id: "views",
      type: "Tabs",
      props: {
        field: "tab",
        items: [
          { value: "delivery", label: "Delivery evidence" },
          { value: "explore", label: "Explore displays" },
        ],
      },
      children: [
        {
          id: "evidence",
          type: "Section",
          props: { title: "Implementation evidence" },
          children: [
            {
              id: "checks",
              type: "List",
              props: {
                items: [
                  "Keep the accepted JSON in the original tool result.",
                  "Read wide tables within their keyboard-accessible region.",
                  "Keep captured labels such as <release> and https://example.test literal.",
                ],
              },
            },
            {
              id: "responsibilities",
              type: "Table",
              props: {
                caption: "Plugin delivery responsibilities",
                rowKey: "id",
                searchState: "search",
                sortState: "sort",
                pageState: "page",
                selectState: "selected",
                pageSize: 2,
                columns: [
                  { key: "capability", label: "Capability" },
                  { key: "owner", label: "Evidence owner" },
                  { key: "input", label: "Accepted input" },
                  { key: "output", label: "Recorded output" },
                  { key: "location", label: "Execution location" },
                  { key: "presentation", label: "Presentation" },
                  { key: "recovery", label: "Recovery behavior" },
                  { key: "decision", label: "Review decision" },
                ],
                rows: [
                  {
                    id: "generated",
                    capability: "Generated report",
                    owner: "GenUI contract",
                    input: "Validated JSON document",
                    output: "Immutable structured content",
                    location: "Managed backend",
                    presentation: "Shared Flame components",
                    recovery: "Original result remains inspectable",
                    decision: "Ready for verification",
                  },
                  {
                    id: "identifiers",
                    capability: "Exact identifiers",
                    owner: "Captured source",
                    input: "9007199254740993",
                    output: "9007199254740993",
                    location: "No numeric conversion",
                    presentation: "Literal metric value",
                    recovery: "Replay the same stored text",
                    decision: "Retain full precision",
                  },
                  {
                    id: "browser",
                    capability: "Browser automation",
                    owner: "Browser session driver",
                    input: "Authorized observation and action",
                    output: "Evidence and original outcome",
                    location: "Explicit Runtime host",
                    presentation: "Separate control surface",
                    recovery: "Re-observe before another action",
                    decision: "Separate plugin work",
                  },
                  {
                    id: "interaction",
                    capability: "Interactive UI",
                    owner: "Runtime execution",
                    input: "Typed declared operation",
                    output: "Original command receipt",
                    location: "Captured installation",
                    presentation: "Local controls and live evidence",
                    recovery: "Recover the original request",
                    decision: "Verify the full interaction",
                  },
                ],
              },
            },
            {
              id: "selectedCount",
              type: "Metric",
              props: {
                label: "Selected evidence rows",
                value: { call: "length", args: [{ state: "selected" }] },
              },
            },
            {
              id: "trajectory",
              type: "Chart",
              props: {
                kind: "line",
                caption: "Delivery checks over time",
                labels: ["Week 1", "Week 2", "Week 3", "Week 4"],
                series: [
                  { label: "Passed", values: [12, 24, 36, 48] },
                  { label: "Reviewed", values: [8, 20, 32, 45] },
                ],
              },
            },
          ],
        },
        {
          id: "exploration",
          type: "Stack",
          props: { direction: "vertical" },
          children: [
            {
              id: "renderedItems",
              type: "Each",
              props: { items: { state: "capabilities" }, as: "capability", limit: 3 },
              children: [
                {
                  id: "capabilityBadge",
                  type: "Badge",
                  props: { text: { item: "capability" }, tone: "accent" },
                },
              ],
            },
            {
              id: "explanation",
              type: "Markdown",
              props: {
                text: "### Composable presentation\n\nA **local** scenario uses the same document controls and expressions. [Open the source](https://github.com/thesysdev/openui).\n\n- No second form store\n- No invented execution result",
              },
            },
            { id: "equation", type: "Math", props: { expression: "c = 12w", display: true } },
            {
              id: "previewSvg",
              type: "Svg",
              props: { label: "Delivery progress illustration", viewBox: "0 0 320 80", height: 80 },
              children: [
                {
                  id: "trackShape",
                  type: "Rect",
                  props: {
                    x: 0,
                    y: 30,
                    width: 300,
                    height: 20,
                    fill: "none",
                    stroke: "currentColor",
                  },
                },
                {
                  id: "progressShape",
                  type: "Rect",
                  props: { x: 0, y: 30, width: 225, height: 20, fill: "currentColor" },
                },
              ],
            },
            { id: "progress", type: "Progress", props: { label: "Review coverage", value: 75 } },
            { id: "separator", type: "Divider", props: {} },
            {
              id: "flashcard",
              type: "Flashcard",
              props: {
                field: "flipped",
                front: "Who owns execution receipts?",
                back: "Runtime owns execution and recovery.",
              },
            },
            {
              id: "reviewTimer",
              type: "Timer",
              props: { field: "timer", runningState: "timerRunning", label: "Review timer" },
            },
            {
              id: "notesPopover",
              type: "Popover",
              props: { field: "popover", label: "Review hint" },
              children: [
                {
                  id: "hint",
                  type: "Text",
                  props: { text: "Check one original outcome before trying an action again." },
                },
              ],
            },
          ],
        },
      ],
    },
  ],
};
