import { NavLink } from "react-router";
import {
  MessageSquare,
  Wrench,
  Bot,
  Sparkles,
  KeyRound,
  LayoutDashboard,
  Clock,
  FileText,
  Settings,
  BookOpen,
  Monitor,
  Heart,
  ChevronRight,
  PanelLeftClose,
} from "lucide-react";

type NavItem = {
  to: string;
  icon: typeof LayoutDashboard;
  label: string;
  featured?: boolean;
};
type NavGroup = { items: NavItem[] };

const navGroups: NavGroup[] = [
  {
    items: [
      {
        to: "/dashboards",
        icon: LayoutDashboard,
        label: "Dashboard",
        featured: true,
      },
    ],
  },
  {
    items: [
      { to: "/chat", icon: MessageSquare, label: "Chats" },
      { to: "/agents", icon: Bot, label: "Agents" },
      { to: "/browser", icon: Monitor, label: "Browsers" },
      { to: "/context", icon: BookOpen, label: "Context" },
    ],
  },
  {
    items: [
      { to: "/scheduler", icon: Clock, label: "Scheduler" },
      { to: "/heartbeat", icon: Heart, label: "Heartbeat" },
    ],
  },
  {
    items: [
      { to: "/tools", icon: Wrench, label: "Tools" },
      { to: "/skills", icon: Sparkles, label: "Skills" },
      { to: "/library", icon: BookOpen, label: "Library" },
    ],
  },
  {
    items: [
      { to: "/secrets", icon: KeyRound, label: "Secrets" },
      { to: "/logs", icon: FileText, label: "Logs" },
    ],
  },
  {
    items: [{ to: "/settings", icon: Settings, label: "Settings" }],
  },
];

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function Sidebar({ collapsed, onToggle }: SidebarProps) {
  return (
    <aside
      className={`hidden md:flex flex-col bg-surface-1 border-r border-border-0 transition-all duration-200 relative z-[1] ${collapsed ? "w-16" : "w-56"}`}
    >
      <nav className="flex-1 py-3 px-2 overflow-y-auto">
        {navGroups.map((group, gi) => (
          <div key={gi}>
            {gi > 0 && <div className="mx-3 my-2 border-b border-border-0" />}
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 rounded-lg text-sm font-medium transition-all duration-150 ${
                      item.featured ? "py-3" : "py-2.5"
                    } ${
                      isActive
                        ? item.featured
                          ? "bg-accent-primary/15 text-accent-text"
                          : "bg-accent-muted text-accent-text"
                        : item.featured
                          ? "text-text-2 hover:text-text-1 hover:bg-surface-2"
                          : "text-text-2 hover:text-text-1 hover:bg-surface-2"
                    } ${collapsed ? "justify-center" : ""}`
                  }
                  title={collapsed ? item.label : undefined}
                >
                  <item.icon className="flex-shrink-0 w-5 h-5" />
                  {!collapsed && (
                    <span className={item.featured ? "font-semibold" : ""}>
                      {item.label}
                    </span>
                  )}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      <div className="border-t border-border-0">
        <button
          onClick={onToggle}
          className={`w-full flex items-center gap-2 p-3 text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer ${collapsed ? "justify-center" : "px-4"}`}
          title={collapsed ? "Expand Sidebar" : undefined}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {collapsed ? (
            <ChevronRight className="w-4 h-4" aria-hidden="true" />
          ) : (
            <>
              <PanelLeftClose className="w-4 h-4" aria-hidden="true" />
              <span className="text-xs">Hide Sidebar</span>
            </>
          )}
        </button>

        {!collapsed && (
          <div className="px-4 pb-3 pt-0">
            <p className="text-[10px] text-text-3" aria-hidden="true">
              &copy; OpenPaw &middot; Agentic Factory
            </p>
          </div>
        )}
      </div>
    </aside>
  );
}
