import type { IconComponent } from "reicon-react/createIcon";
import { memo, type CSSProperties } from "react";
import type { IconSize } from "@/lib/iconScale";
import { AlertTriangle } from "reicon-react/icons/AlertTriangle";
import { Archive } from "reicon-react/icons/Archive";
import { ArrowDown } from "reicon-react/icons/ArrowDown";
import { ArrowLeft } from "reicon-react/icons/ArrowLeft";
import { ArrowRight } from "reicon-react/icons/ArrowRight";
import { ArrowSwapHorizontal } from "reicon-react/icons/ArrowSwapHorizontal";
import { ArrowUp } from "reicon-react/icons/ArrowUp";
import { Backward } from "reicon-react/icons/Backward";
import { Bell } from "reicon-react/icons/Bell";
import { Book } from "reicon-react/icons/Book";
import { BookOpen } from "reicon-react/icons/BookOpen";
import { BoxSearch } from "reicon-react/icons/BoxSearch";
import { Bug } from "reicon-react/icons/Bug";
import { Bullseye } from "reicon-react/icons/Bullseye";
import { CalendarPlus } from "reicon-react/icons/CalendarPlus";
import { CalendarX } from "reicon-react/icons/CalendarX";
import { Chart } from "reicon-react/icons/Chart";
import { Chat } from "reicon-react/icons/Chat";
import { Check } from "reicon-react/icons/Check";
import { ChevronDown } from "reicon-react/icons/ChevronDown";
import { ChevronLeft } from "reicon-react/icons/ChevronLeft";
import { ChevronRight } from "reicon-react/icons/ChevronRight";
import { ChevronUp } from "reicon-react/icons/ChevronUp";
import { CircleTransferH } from "reicon-react/icons/CircleTransferH";
import { ClipboardCheck } from "reicon-react/icons/ClipboardCheck";
import { Clock } from "reicon-react/icons/Clock";
import { CloudConnection } from "reicon-react/icons/CloudConnection";
import { Code } from "reicon-react/icons/Code";
import { Command } from "reicon-react/icons/Command";
import { Copy } from "reicon-react/icons/Copy";
import { Cpu } from "reicon-react/icons/Cpu";
import { Database } from "reicon-react/icons/Database";
import { Designtools } from "reicon-react/icons/Designtools";
import { Download } from "reicon-react/icons/Download";
import { Eye } from "reicon-react/icons/Eye";
import { File } from "reicon-react/icons/File";
import { FilePlus } from "reicon-react/icons/FilePlus";
import { FileText } from "reicon-react/icons/FileText";
import { Files } from "reicon-react/icons/Files";
import { FilterSearch } from "reicon-react/icons/FilterSearch";
import { Flag } from "reicon-react/icons/Flag";
import { Folder } from "reicon-react/icons/Folder";
import { FolderOpen } from "reicon-react/icons/FolderOpen";
import { Forward } from "reicon-react/icons/Forward";
import { Globe } from "reicon-react/icons/Globe";
import { Help } from "reicon-react/icons/Help";
import { Hierarchy } from "reicon-react/icons/Hierarchy";
import { History } from "reicon-react/icons/History";
import { Image } from "reicon-react/icons/Image";
import { Library } from "reicon-react/icons/Library";
import { Lightning } from "reicon-react/icons/Lightning";
import { List } from "reicon-react/icons/List";
import { ListCheck } from "reicon-react/icons/ListCheck";
import { Map } from "reicon-react/icons/Map";
import { Maximize } from "reicon-react/icons/Maximize";
import { Minimize } from "reicon-react/icons/Minimize";
import { Moon } from "reicon-react/icons/Moon";
import { MoreH } from "reicon-react/icons/MoreH";
import { Paperclip } from "reicon-react/icons/Paperclip";
import { Pause } from "reicon-react/icons/Pause";
import { Pen } from "reicon-react/icons/Pen";
import { Play } from "reicon-react/icons/Play";
import { Plus } from "reicon-react/icons/Plus";
import { Receipt } from "reicon-react/icons/Receipt";
import { Repeat } from "reicon-react/icons/Repeat";
import { Routing } from "reicon-react/icons/Routing";
import { Search } from "reicon-react/icons/Search";
import { SearchStatus } from "reicon-react/icons/SearchStatus";
import { SearchZoomIn } from "reicon-react/icons/SearchZoomIn";
import { SearchZoomOut } from "reicon-react/icons/SearchZoomOut";
import { Send } from "reicon-react/icons/Send";
import { Settings } from "reicon-react/icons/Settings";
import { Share } from "reicon-react/icons/Share";
import { Shield } from "reicon-react/icons/Shield";
import { SidebarLeft } from "reicon-react/icons/SidebarLeft";
import { SidebarRight } from "reicon-react/icons/SidebarRight";
import { Sparkle } from "reicon-react/icons/Sparkle";
import { Sparkles } from "reicon-react/icons/Sparkles";
import { Star } from "reicon-react/icons/Star";
import { Stop } from "reicon-react/icons/Stop";
import { Sun } from "reicon-react/icons/Sun";
import { Target } from "reicon-react/icons/Target";
import { TerminalSquare } from "reicon-react/icons/TerminalSquare";
import { TextalignLeft } from "reicon-react/icons/TextalignLeft";
import { ThumbsDown } from "reicon-react/icons/ThumbsDown";
import { ThumbsUp } from "reicon-react/icons/ThumbsUp";
import { Trash } from "reicon-react/icons/Trash";
import { User } from "reicon-react/icons/User";
import { Users } from "reicon-react/icons/Users";
import { X } from "reicon-react/icons/X";

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
  | "panel-r"
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
  | "wrap-text";

const ICON_MAP = {
  search: Search,
  plus: Plus,
  "zoom-in": SearchZoomIn,
  "zoom-out": SearchZoomOut,
  chat: Chat,
  folder: Folder,
  "folder-open": FolderOpen,
  code: Code,
  terminal: TerminalSquare,
  file: File,
  filetext: FileText,
  send: Send,
  "send-arrow": ArrowUp,
  stop: Stop,
  play: Play,
  pause: Pause,
  settings: Settings,
  sun: Sun,
  moon: Moon,
  share: Share,
  more: MoreH,
  x: X,
  check: Check,
  branch: Hierarchy,
  git: Routing,
  globe: Globe,
  book: Book,
  history: History,
  tool: Designtools,
  sparkle: Sparkle,
  "thumbs-up": ThumbsUp,
  "thumbs-down": ThumbsDown,
  edit: Pen,
  image: Image,
  command: Command,
  panel: SidebarRight,
  "panel-l": SidebarLeft,
  "panel-r": SidebarRight,
  user: User,
  spark: Sparkles,
  "skip-back": Backward,
  "skip-fwd": Forward,
  minimize: Minimize,
  maximize: Maximize,
  diff: Files,
  list: List,
  chart: Chart,
  clock: Clock,
  bell: Bell,
  lightning: Lightning,
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
  trash: Trash,
  alert: AlertTriangle,
  eye: Eye,
  "file-plus": FilePlus,
  "folder-search": BoxSearch,
  download: Download,
  bot: Cpu,
  question: Help,
  star: Star,
  scroll: Receipt,
  replace: CircleTransferH,
  "text-search": SearchStatus,
  webhook: CloudConnection,
  library: Library,
  "book-open": BookOpen,
  paperclip: Paperclip,
  users: Users,
  map: Map,
  "list-checks": ListCheck,
  flag: Flag,
  brain: Database,
  "package-search": FilterSearch,
  archive: Archive,
  "calendar-plus": CalendarPlus,
  "calendar-x": CalendarX,
  target: Target,
  crosshair: Bullseye,
  "clipboard-check": ClipboardCheck,
  "unfold-horizontal": ArrowSwapHorizontal,
  "wrap-text": TextalignLeft,
} satisfies Record<IconName, IconComponent>;

export const ICON_NAMES: ReadonlySet<IconName> = new Set(Object.keys(ICON_MAP) as IconName[]);

/**
 * Narrow a contributed string to a glyph this set actually draws.
 *
 * Icon names reach us as plain strings — from plugin contributions, from a workspace view or
 * settings pane spec, and from MCP server data the Runtime forwards. Casting one straight to
 * `IconName` type-checks and then draws NOTHING for a name we do not have: no error, no
 * fallback, just a gap where a glyph belongs. The cast is honest here because `has` earned it,
 * and every caller is left to say what it wants shown instead.
 */
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

// Memoised, and the default style is hoisted, because reicon paints every glyph through
// `dangerouslySetInnerHTML`: a re-render with a changed prop tears out the <path> nodes and
// builds new ones. When that lands between mousedown and mouseup — IconButton re-renders on
// pointerenter — the browser sees its mousedown target detached and fires NO click, so the
// button silently does nothing. Stable props keep the DOM nodes alive through a press.
export const Icon = memo(function Icon({ name, size = "sm", style, className }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      aria-hidden="true"
      data-icon-name={name}
      className={className}
      style={style ? { ...SIZE_STYLE[size], ...style } : SIZE_STYLE[size]}
    />
  );
});
