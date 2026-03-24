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
import { useT } from '@/i18n';
import type { TranslationKey } from '@/i18n';
import type { LucideIcon } from 'lucide-react';

const navItems: { to: string; labelKey: TranslationKey; icon: LucideIcon; end?: boolean }[] = [
  { to: '/', labelKey: 'nav.overview', icon: LayoutDashboard, end: true },
  { to: '/proxies', labelKey: 'nav.proxies', icon: Server },
  { to: '/proxy-groups', labelKey: 'nav.proxyGroups', icon: Layers },
  { to: '/rules', labelKey: 'nav.rules', icon: Shield },
  { to: '/rule-providers', labelKey: 'nav.ruleProviders', icon: Database },
  { to: '/system-config', labelKey: 'nav.systemConfig', icon: Settings2 },
  { to: '/publish', labelKey: 'nav.publish', icon: Rocket },
  { to: '/runtime', labelKey: 'nav.runtime', icon: Activity },
  { to: '/settings', labelKey: 'nav.settings', icon: Settings },
];

export function Sidebar() {
  const t = useT();

  return (
    <aside className="w-64 flex-shrink-0 border-r bg-background flex flex-col">
      <div className="h-14 flex items-center px-5 border-b">
        <span className="font-semibold text-base tracking-tight">{t('app.name')}</span>
      </div>
      <nav className="flex-1 overflow-y-auto py-3 px-3 space-y-0.5">
        {navItems.map(({ to, labelKey, icon: Icon, end }) => (
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
            {t(labelKey)}
          </NavLink>
        ))}
      </nav>
    </aside>
  );
}
