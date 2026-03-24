import { useEffect, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { runtimeApi } from '@/api/runtime';
import { proxiesApi } from '@/api/proxies';
import { rulesApi } from '@/api/rules';
import { proxyGroupsApi } from '@/api/proxy-groups';
import { publishApi, type PublishRecord } from '@/api/publish';
import { useT } from '@/i18n';

interface StatCard {
  label: string;
  value: string | number;
  sub?: string;
}

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  success: 'default',
  failed: 'destructive',
  rolled_back: 'secondary',
};

function formatRelTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const min = Math.floor(diff / 60000);
  if (min < 1) return 'just now';
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

export default function OverviewPage() {
  const t = useT();
  const [mihomoVersion, setMihomoVersion] = useState<string | null>(null);
  const [mihomoReachable, setMihomoReachable] = useState<boolean | null>(null);
  const [proxyCount, setProxyCount] = useState<number | null>(null);
  const [ruleCount, setRuleCount] = useState<number | null>(null);
  const [groupCount, setGroupCount] = useState<number | null>(null);
  const [connectionCount, setConnectionCount] = useState<number | null>(null);
  const [recentPublishes, setRecentPublishes] = useState<PublishRecord[]>([]);

  useEffect(() => {
    // mihomo version
    runtimeApi
      .version()
      .then((res) => {
        const v = res?.data?.version ?? res?.data;
        setMihomoVersion(typeof v === 'string' ? v : 'unknown');
        setMihomoReachable(true);
      })
      .catch(() => {
        setMihomoReachable(false);
      });

    // proxy count
    proxiesApi
      .list()
      .then((res) => setProxyCount((res.data ?? []).length))
      .catch(() => setProxyCount(null));

    // rule count
    rulesApi
      .list()
      .then((res) => setRuleCount((res.data ?? []).length))
      .catch(() => setRuleCount(null));

    // group count
    proxyGroupsApi
      .list()
      .then((res) => setGroupCount((res.data ?? []).length))
      .catch(() => setGroupCount(null));

    // active connections
    runtimeApi
      .connections()
      .then((res) => {
        const data = res?.data;
        if (data && typeof data === 'object' && Array.isArray(data.connections)) {
          setConnectionCount(data.connections.length);
        } else if (Array.isArray(data)) {
          setConnectionCount(data.length);
        } else {
          setConnectionCount(0);
        }
      })
      .catch(() => setConnectionCount(null));

    // recent publishes
    publishApi
      .history(5)
      .then((res) => setRecentPublishes(res.data ?? []))
      .catch(() => setRecentPublishes([]));
  }, []);

  const stats: StatCard[] = [
    {
      label: t('overview.mihomoStatus'),
      value:
        mihomoReachable === null
          ? t('overview.checking')
          : mihomoReachable
            ? mihomoVersion ?? 'Reachable'
            : t('overview.unreachable'),
      sub: mihomoReachable ? t('overview.running') : mihomoReachable === false ? t('overview.cannotConnect') : undefined,
    },
    {
      label: t('overview.proxies'),
      value: proxyCount ?? '—',
      sub: t('overview.configured'),
    },
    {
      label: t('nav.rules'),
      value: ruleCount ?? '—',
      sub: t('overview.configured'),
    },
    {
      label: t('overview.groups'),
      value: groupCount ?? '—',
      sub: t('overview.proxyGroups'),
    },
    {
      label: t('overview.activeConnections'),
      value: connectionCount ?? '—',
      sub: mihomoReachable === false ? t('overview.mihomoUnreachable') : t('overview.live'),
    },
  ];

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t('overview.title')}</h1>

      {/* Stats grid */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        {stats.map((stat) => (
          <Card key={stat.label}>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                {stat.label}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-bold">{stat.value}</p>
              {stat.sub && <p className="text-xs text-muted-foreground mt-0.5">{stat.sub}</p>}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Recent publishes */}
      <Card>
        <CardHeader className="border-b pb-3">
          <CardTitle>{t('overview.recentPublishes')}</CardTitle>
        </CardHeader>
        <CardContent className="pt-0">
          {recentPublishes.length === 0 ? (
            <p className="text-center py-8 text-sm text-muted-foreground">{t('overview.noPublishHistory')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('overview.version')}</TableHead>
                  <TableHead>{t('overview.status')}</TableHead>
                  <TableHead>{t('overview.operator')}</TableHead>
                  <TableHead>{t('overview.note')}</TableHead>
                  <TableHead>{t('overview.time')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recentPublishes.map((record) => (
                  <TableRow key={record.id}>
                    <TableCell className="font-medium">v{record.version}</TableCell>
                    <TableCell>
                      <Badge variant={STATUS_VARIANT[record.status] ?? 'outline'}>
                        {record.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm">{record.operator || '—'}</TableCell>
                    <TableCell className="text-sm text-muted-foreground max-w-[200px] truncate">
                      {record.note || '—'}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatRelTime(record.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
