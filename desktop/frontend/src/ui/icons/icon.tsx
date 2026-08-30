import type { LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import type { IconSize } from "@/lib/iconScale";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Bell,
  Book,
  Bot,
  Bug,
  ChartColumn,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  CircleHelp,
  Clock,
  Code,
  Command,
  Copy,
  Download,
  Ellipsis,
  Eye,
  File,
  FileDiff,
  FilePlus,
  FileText,
  Folder,
  FolderOpen,
  FolderSearch,
  GitBranch,
  GitFork,
  Globe,
  Image as ImageIcon,
  History,
  List,
  Maximize2,
  MessageSquare,
  Minimize2,
  Moon,
  PanelLeft,
  PanelRight,
  Pause,
  Pencil,
  Play,
  Plus,
  Repeat,
  Search,
  Send,
  Settings,
  Share2,
  Shield,
  SkipBack,
  SkipForward,
  Sparkle,
  Sparkles,
  Square,
  Star,
  Sun,
  Terminal,
  ThumbsDown,
  ThumbsUp,
  Trash2,
  TriangleAlert,
  UnfoldHorizontal,
  User,
  WrapText,
  Wrench,
  X,
  ZoomIn,
  ZoomOut,
  Zap,
  Archive,
  BookOpen,
  Brain,
  CalendarPlus,
  CalendarX,
  ClipboardCheck,
  Crosshair,
  Flag,
  Library,
  ListChecks,
  Map as MapIcon,
  PackageSearch,
  Paperclip,
  Replace,
  ScrollText,
  Target,
  TextSearch,
  Users,
  Webhook,
} from "lucide-react";

// Plugins consume this vocabulary rather than lucide component names directly, so the
// bundle only ships glyphs named here and a glyph can be re-pointed in one place.

export type IconName =
  | "search"
  | "plus"
  | "chat"
  | "folder"
  | "folder-open"
  | "code"
  | "terminal"
  | "file"
  | "filetext"
  | "send"
  | "send-arrow"
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
  | "git"
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
  | "panel"
  | "panel-l"
  | "user"
  | "spark"
  | "skip-back"
  | "skip-fwd"
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
  | "panel-r"
  | "arrow-down"
  | "arrow-left"
  | "arrow-right"
  | "arrow-up"
  | "trash"
  | "alert"
  | "eye"
  | "file-plus"
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
  | "wrap-text"
  | "zoom-in"
  | "zoom-out";

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
  "send-arrow": ArrowUp,
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
  git: GitFork,
  globe: Globe,
  book: Book,
  history: History,
  tool: Wrench,
  sparkle: Sparkle,
  "thumbs-up": ThumbsUp,
  "thumbs-down": ThumbsDown,
  edit: Pencil,
  image: ImageIcon,
  command: Command,
  panel: PanelRight,
  "panel-l": PanelLeft,
  "panel-r": PanelRight,
  user: User,
  spark: Sparkles,
  "skip-back": SkipBack,
  "skip-fwd": SkipForward,
  minimize: Minimize2,
  maximize: Maximize2,
  diff: FileDiff,
  list: List,
  chart: ChartColumn,
  clock: Clock,
  bell: Bell,
  lightning: Zap,
  bug: Bug,
  shield: Shield,
  loop: Repeat,
  copy: Copy,
  "chevron-up": ChevronUp,
  "chevron-down": ChevronDown,
  "chevron-left": ChevronLeft,
  "chevron-right": ChevronRight,
  "arrow-down": ArrowDown,
  "arrow-left": ArrowLeft,
  "arrow-right": ArrowRight,
  "arrow-up": ArrowUp,
  trash: Trash2,
  alert: TriangleAlert,
  eye: Eye,
  "file-plus": FilePlus,
  "folder-search": FolderSearch,
  download: Download,
  bot: Bot,
  question: CircleHelp,
  star: Star,
  scroll: ScrollText,
  replace: Replace,
  "text-search": TextSearch,
  webhook: Webhook,
  library: Library,
  "book-open": BookOpen,
  paperclip: Paperclip,
  users: Users,
  map: MapIcon,
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
  "wrap-text": WrapText,
} satisfies Record<IconName, LucideIcon>;

/**
 * The vocabulary as data, for tables that name a glyph in a plain string. `Icon` itself is
 * typed; a registry contribution is `Record<string, string>`, so its glyph names are
 * checked by the test that reads this rather than by the compiler.
 */
export const ICON_NAMES: ReadonlySet<IconName> = new Set(Object.keys(ICON_MAP) as IconName[]);

interface Props {
  name: IconName;
  size?: IconSize;
  style?: CSSProperties;
  className?: string;
}

// Geometry rides CSS custom properties rather than props so a change to the user's base
// size reaches every glyph without a re-render, and so stroke width cannot drift away from
// the size that derives it.
export function Icon({ name, size = "sm", style, className }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      aria-hidden="true"
      className={className}
      style={{
        width: `var(--icon-${size})`,
        height: `var(--icon-${size})`,
        strokeWidth: `var(--icon-stroke-${size})`,
        ...style,
      }}
    />
  );
}
