import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Server,
  Layers,
  Shield,
  Database,
  Settings2,
  Rocket,
  Activity,
  Settings,
} from 'lucide-react';
import { cn } from '@/lib/utils';

const navItems = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/proxies', label: 'Proxies', icon: Server },
  { to: '/proxy-groups', label: 'Proxy Groups', icon: Layers },
  { to: '/rules', label: 'Rules', icon: Shield },
  { to: '/rule-providers', label: 'Rule Providers', icon: Database },
  { to: '/system-config', label: 'System Config', icon: Settings2 },
  { to: '/publish', label: 'Publish', icon: Rocket },
  { to: '/runtime', label: 'Runtime', icon: Activity },
  { to: '/settings', label: 'Settings', icon: Settings },
];

export function Sidebar() {
  return (
    <aside className="w-64 flex-shrink-0 border-r bg-background flex flex-col">
      <div className="h-14 flex items-center px-5 border-b">
        <span className="font-semibold text-base tracking-tight">Mihomo CP</span>
      </div>
      <nav className="flex-1 overflow-y-auto py-3 px-3 space-y-0.5">
        {navItems.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
              )
            }
          >
            <Icon className="h-4 w-4 shrink-0" />
            {label}
          </NavLink>
        ))}
      </nav>
    </aside>
  );
}
