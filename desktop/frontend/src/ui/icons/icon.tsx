import type { LucideIcon } from "lucide-react";
import { memo, type CSSProperties } from "react";
import type { IconSize } from "@/lib/iconScale";
import {
  Activity,
  Archive,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  ArrowUpRight,
  Bell,
  Blocks,
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
  Columns2,
  Command,
  Copy,
  CopyX,
  Crosshair,
  Diff,
  Download,
  Ellipsis,
  Eye,
  File,
  FileText,
  Flag,
  FoldVertical,
  Folder,
  FolderOpen,
  FolderSearch,
  Gauge,
  GitBranch,
  Globe,
  Image,
  Library,
  List,
  ListChecks,
  ListX,
  Map,
  Maximize,
  MessageSquare,
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
  RotateCcwClock,
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
  SquarePen,
  Star,
  Sun,
  Target,
  Terminal,
  TextSearch,
  TextWrap,
  ThumbsDown,
  ThumbsUp,
  Trash,
  TriangleAlert,
  UnfoldHorizontal,
  User,
  Users,
  Webhook,
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
  | "fold"
  | "columns"
  | "gauge"
  | "open"
  | "compose"
  | "close-others"
  | "close-all"
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
  | "activity"
  | "blocks"
  | "target"
  | "crosshair"
  | "clipboard-check"
  | "unfold-horizontal"
  | "wrap-text";

const ICON_MAP = {
  activity: Activity,
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
  fold: FoldVertical,
  columns: Columns2,
  gauge: Gauge,
  open: ArrowUpRight,
  compose: SquarePen,
  "close-others": CopyX,
  "close-all": ListX,
  maximize: Maximize,
  diff: Diff,
  list: List,
  chart: ChartColumn,
  clock: Clock,
  bell: Bell,
  blocks: Blocks,
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
  trash: Trash,
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

export function knownIconName(value: string | null | undefined): IconName | undefined {
  return value != null && ICON_NAMES.has(value as IconName) ? (value as IconName) : undefined;
}

interface Props {
  name: IconName;
  size?: IconSize;
  style?: CSSProperties;
  className?: string;
  full?: boolean;
}

const SIZE_STYLE = Object.fromEntries(
  (["xs", "sm", "md", "lg", "xl"] as const).map((size) => [
    size,
    {
      width: `var(--icon-${size})`,
      height: `var(--icon-${size})`,
      strokeWidth: `var(--icon-stroke-${size})`,
    },
  ]),
) as Readonly<Record<IconSize, CSSProperties>>;

export const Icon = memo(function Icon({ name, size = "sm", style, className, full }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      aria-hidden="true"
      data-icon-name={name}
      data-glyph={full ? "full" : undefined}
      className={className}
      style={style ? { ...SIZE_STYLE[size], ...style } : SIZE_STYLE[size]}
    />
  );
});
