import type { LucideIcon } from "lucide-react";
import { memo, type CSSProperties } from "react";
import type { IconSize } from "@/lib/iconScale";
import {
  Archive,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Bell,
  Book,
  BookOpen,
  Bot,
  Brain,
  Bug,
  CalendarPlus,
  CalendarX,
  ChartColumn,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  CircleQuestionMark,
  ClipboardCheck,
  Clock,
  Code,
  Command,
  Copy,
  Crosshair,
  Download,
  Ellipsis,
  Eye,
  File,
  FileDiff,
  FileText,
  Flag,
  Folder,
  FolderOpen,
  FolderSearch,
  GitBranch,
  Globe,
  RotateCcwClock,
  Image,
  Library,
  List,
  ListChecks,
  Map,
  Maximize,
  MessageSquare,
  Minimize,
  Moon,
  PackageSearch,
  PanelLeft,
  PanelRight,
  Paperclip,
  Pause,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Replace,
  ScrollText,
  Search,
  Send,
  Settings,
  Share2,
  ShieldCheck,
  SkipBack,
  Sparkle,
  Sparkles,
  Square,
  Star,
  Sun,
  Target,
  Terminal,
  TextSearch,
  ThumbsDown,
  ThumbsUp,
  Trash2,
  TriangleAlert,
  UnfoldHorizontal,
  User,
  Users,
  Webhook,
  TextWrap,
  Wrench,
  X,
  Zap,
  ZoomIn,
  ZoomOut,
} from "lucide-react";

export type IconName =
  | "search"
  | "plus"
  | "zoom-in"
  | "zoom-out"
  | "chat"
  | "folder"
  | "folder-open"
  | "code"
  | "terminal"
  | "file"
  | "filetext"
  | "send"
  | "stop"
  | "play"
  | "pause"
  | "settings"
  | "sun"
  | "moon"
  | "share"
  | "more"
  | "x"
  | "check"
  | "branch"
  | "globe"
  | "book"
  | "history"
  | "tool"
  | "sparkle"
  | "thumbs-up"
  | "thumbs-down"
  | "edit"
  | "image"
  | "command"
  | "panel-l"
  | "panel-r"
  | "user"
  | "spark"
  | "skip-back"
  | "minimize"
  | "maximize"
  | "diff"
  | "list"
  | "chart"
  | "clock"
  | "bell"
  | "lightning"
  | "bug"
  | "shield"
  | "loop"
  | "copy"
  | "chevron-up"
  | "chevron-down"
  | "chevron-left"
  | "chevron-right"
  | "arrow-left"
  | "arrow-right"
  | "arrow-up"
  | "trash"
  | "alert"
  | "eye"
  | "folder-search"
  | "download"
  | "bot"
  | "question"
  | "star"
  | "scroll"
  | "replace"
  | "text-search"
  | "webhook"
  | "library"
  | "book-open"
  | "paperclip"
  | "users"
  | "map"
  | "list-checks"
  | "flag"
  | "brain"
  | "package-search"
  | "archive"
  | "calendar-plus"
  | "calendar-x"
  | "target"
  | "crosshair"
  | "clipboard-check"
  | "unfold-horizontal"
  | "wrap-text";

const ICON_MAP = {
  search: Search,
  plus: Plus,
  "zoom-in": ZoomIn,
  "zoom-out": ZoomOut,
  chat: MessageSquare,
  folder: Folder,
  "folder-open": FolderOpen,
  code: Code,
  terminal: Terminal,
  file: File,
  filetext: FileText,
  send: Send,
  stop: Square,
  play: Play,
  pause: Pause,
  settings: Settings,
  sun: Sun,
  moon: Moon,
  share: Share2,
  more: Ellipsis,
  x: X,
  check: Check,
  branch: GitBranch,
  globe: Globe,
  book: Book,
  history: RotateCcwClock,
  tool: Wrench,
  sparkle: Sparkle,
  "thumbs-up": ThumbsUp,
  "thumbs-down": ThumbsDown,
  edit: Pencil,
  image: Image,
  command: Command,
  "panel-l": PanelLeft,
  "panel-r": PanelRight,
  user: User,
  spark: Sparkles,
  "skip-back": SkipBack,
  minimize: Minimize,
  maximize: Maximize,
  diff: FileDiff,
  list: List,
  chart: ChartColumn,
  clock: Clock,
  bell: Bell,
  lightning: Zap,
  bug: Bug,
  shield: ShieldCheck,
  loop: RefreshCw,
  copy: Copy,
  "chevron-up": ChevronUp,
  "chevron-down": ChevronDown,
  "chevron-left": ChevronLeft,
  "chevron-right": ChevronRight,
  "arrow-left": ArrowLeft,
  "arrow-right": ArrowRight,
  "arrow-up": ArrowUp,
  trash: Trash2,
  alert: TriangleAlert,
  eye: Eye,
  "folder-search": FolderSearch,
  download: Download,
  bot: Bot,
  question: CircleQuestionMark,
  star: Star,
  scroll: ScrollText,
  replace: Replace,
  "text-search": TextSearch,
  webhook: Webhook,
  library: Library,
  "book-open": BookOpen,
  paperclip: Paperclip,
  users: Users,
  map: Map,
  "list-checks": ListChecks,
  flag: Flag,
  brain: Brain,
  "package-search": PackageSearch,
  archive: Archive,
  "calendar-plus": CalendarPlus,
  "calendar-x": CalendarX,
  target: Target,
  crosshair: Crosshair,
  "clipboard-check": ClipboardCheck,
  "unfold-horizontal": UnfoldHorizontal,
  "wrap-text": TextWrap,
} satisfies Record<IconName, LucideIcon>;

export const ICON_NAMES: ReadonlySet<IconName> = new Set(Object.keys(ICON_MAP) as IconName[]);

/** Narrows a contributed string to a glyph this set draws. Casting instead type-checks and
 *  then renders nothing — no error, no fallback. */
export function knownIconName(value: string | null | undefined): IconName | undefined {
  return value != null && ICON_NAMES.has(value as IconName) ? (value as IconName) : undefined;
}

interface Props {
  name: IconName;
  size?: IconSize;
  style?: CSSProperties;
  className?: string;
}

const SIZE_STYLE = Object.fromEntries(
  (["xs", "sm", "md", "lg", "xl"] as const).map((size) => [
    size,
    { width: `var(--icon-${size})`, height: `var(--icon-${size})` },
  ]),
) as Readonly<Record<IconSize, CSSProperties>>;

// Memoised because a transcript renders one of these per tool row, per message action and per
// index row, and re-renders them on every streamed token. The props are four scalars and one
// hoisted style object, so the comparison is cheap and almost always says no.
export const Icon = memo(function Icon({ name, size = "sm", style, className }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      aria-hidden="true"
      data-icon-name={name}
      className={className}
      // Lucide draws on a 24 grid at stroke 2, so the stroke scales with the box: ~1.2px at
      // the 14px step, ~1px at 12. Pinning it (`absoluteStrokeWidth`) would make a 12px glyph
      // carry the same weight as a 28px one, which is what makes small icons read as blobs.
      style={style ? { ...SIZE_STYLE[size], ...style } : SIZE_STYLE[size]}
    />
  );
});
