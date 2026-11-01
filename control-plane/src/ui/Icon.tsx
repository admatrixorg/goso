import type { CSSProperties, SVGProps } from "react";
import {
  Activity,
  ArchiveRestore,
  ArrowLeftRight,
  ArrowUp,
  AudioLines,
  Bell,
  Bookmark,
  Bot,
  Brain,
  Building2,
  Calendar,
  CalendarClock,
  Check,
  ChevronRight,
  Clock,
  Cloud,
  Contact,
  Cpu,
  Download,
  Eye,
  FileText,
  Filter,
  Flag,
  Flame,
  Gift,
  GitBranch,
  Gavel,
  HardDrive,
  History,
  Hourglass,
  Images,
  Inbox,
  KeyRound,
  Languages,
  Layers,
  LayoutDashboard,
  List,
  Lock,
  Megaphone,
  MessagesSquare,
  Monitor,
  Moon,
  Mouse,
  Network,
  Package,
  Plug,
  PlugZap,
  Plus,
  Radio,
  RefreshCw,
  Rss,
  Scale,
  ScanSearch,
  ScatterChart,
  ScrollText,
  Search,
  Settings,
  Share2,
  Shield,
  ShieldCheck,
  Sparkles,
  Star,
  Sun,
  Tag,
  Target,
  Timer,
  TrendingUp,
  Trophy,
  User,
  UserCheck,
  Users,
  Vault,
  Video,
  Webhook,
  Wrench,
  Zap,
  type LucideIcon,
} from "lucide-react";

/** One Lucide icon per name. Stroke is 1.75 in the 24px viewBox. */
export type IconName =
  | "funnel" | "refresh" | "download" | "flag" | "hourglass" | "user-check" | "trophy"
  | "gauge" | "scale" | "pulse" | "layers" | "cloud" | "history" | "scatter" | "timer"
  | "sitemap" | "list" | "dash" | "msg" | "friends" | "user" | "cal" | "gallery" | "mega"
  | "report" | "gear" | "search" | "bell" | "flame" | "eye" | "bolt" | "tag" | "bookmark"
  | "star" | "target" | "trend" | "check" | "clock" | "gift" | "shield" | "inbox" | "plus"
  | "device" | "hook" | "doc" | "lock" | "build" | "sun" | "mouse" | "arrow-up" | "chev-right" | "mic"
  | "layout" | "messages" | "bot" | "contact" | "radio" | "plug" | "network" | "monitor"
  | "sparkles" | "gavel" | "branch" | "wrench" | "plug-zap" | "waveform" | "calendar-clock"
  | "webhook" | "brain" | "vault" | "video" | "share" | "hard-drive" | "scan" | "rss"
  | "activity" | "scroll" | "building" | "cpu" | "key" | "package" | "settings"
  | "shield-check" | "swap" | "archive" | "languages" | "moon"
  | "";

export const ICONS: Record<Exclude<IconName, "">, LucideIcon> = {
  funnel: Filter,
  refresh: RefreshCw,
  download: Download,
  flag: Flag,
  hourglass: Hourglass,
  "user-check": UserCheck,
  trophy: Trophy,
  gauge: LayoutDashboard,
  scale: Scale,
  pulse: Activity,
  layers: Layers,
  cloud: Cloud,
  history: History,
  scatter: ScatterChart,
  timer: Timer,
  sitemap: Share2,
  list: List,
  dash: LayoutDashboard,
  msg: MessagesSquare,
  friends: Users,
  user: User,
  cal: Calendar,
  gallery: Images,
  mega: Megaphone,
  report: ScatterChart,
  gear: Settings,
  search: Search,
  bell: Bell,
  flame: Flame,
  eye: Eye,
  bolt: Zap,
  tag: Tag,
  bookmark: Bookmark,
  star: Star,
  target: Target,
  trend: TrendingUp,
  check: Check,
  clock: Clock,
  gift: Gift,
  shield: Shield,
  inbox: Inbox,
  plus: Plus,
  device: Radio,
  hook: Webhook,
  doc: FileText,
  lock: Lock,
  build: Wrench,
  sun: Sun,
  mouse: Mouse,
  "arrow-up": ArrowUp,
  "chev-right": ChevronRight,
  mic: AudioLines,
  layout: LayoutDashboard,
  messages: MessagesSquare,
  bot: Bot,
  contact: Contact,
  radio: Radio,
  plug: Plug,
  network: Network,
  monitor: Monitor,
  sparkles: Sparkles,
  gavel: Gavel,
  branch: GitBranch,
  wrench: Wrench,
  "plug-zap": PlugZap,
  waveform: AudioLines,
  "calendar-clock": CalendarClock,
  webhook: Webhook,
  brain: Brain,
  vault: Vault,
  video: Video,
  share: Share2,
  "hard-drive": HardDrive,
  scan: ScanSearch,
  rss: Rss,
  activity: Activity,
  scroll: ScrollText,
  building: Building2,
  cpu: Cpu,
  key: KeyRound,
  package: Package,
  settings: Settings,
  "shield-check": ShieldCheck,
  swap: ArrowLeftRight,
  archive: ArchiveRestore,
  languages: Languages,
  moon: Moon,
};

export const ICON_STROKE = 1.75;

export function Icon({
  name,
  size = 16,
  strokeWidth = ICON_STROKE,
  style,
  ...rest
}: { name: IconName; size?: number; strokeWidth?: number } & SVGProps<SVGSVGElement>) {
  if (!name) return null;
  const Glyph = ICONS[name];
  const box: CSSProperties = {
    flex: "none",
    display: "block",
    shapeRendering: "geometricPrecision",
    ...style,
  };
  return (
    <Glyph
      size={size}
      strokeWidth={strokeWidth}
      color="currentColor"
      absoluteStrokeWidth={false}
      aria-hidden="true"
      style={box}
      {...rest}
    />
  );
}
