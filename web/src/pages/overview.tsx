import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ActivityIcon, LoaderIcon, XIcon, RefreshCwIcon, PlusIcon } from 'lucide-react';
import { runtimeApi } from '@/api/runtime';
import { proxiesApi } from '@/api/proxies';
import type { Proxy } from '@/api/proxies';
import { rulesApi, type Rule } from '@/api/rules';
import { RuleForm } from '@/components/rules/rule-form';
import { useT } from '@/i18n';

interface Connection {
  id: string;
  metadata: {
    host?: string;
    destinationIP?: string;
    destinationPort?: string;
    network?: string;
    type?: string;
    sourceIP?: string;
    sourcePort?: string;
  };
  chains?: string[];
  rule?: string;
  rulePayload?: string;
  upload?: number;
  download?: number;
  start?: string;
}

const COUNTRY_FLAG: Record<string, string> = {
  HK: '\u{1F1ED}\u{1F1F0}', TW: '\u{1F1F9}\u{1F1FC}', JP: '\u{1F1EF}\u{1F1F5}',
  KR: '\u{1F1F0}\u{1F1F7}', SG: '\u{1F1F8}\u{1F1EC}', US: '\u{1F1FA}\u{1F1F8}',
  GB: '\u{1F1EC}\u{1F1E7}', DE: '\u{1F1E9}\u{1F1EA}', FR: '\u{1F1EB}\u{1F1F7}',
  CA: '\u{1F1E8}\u{1F1E6}', AU: '\u{1F1E6}\u{1F1FA}', IN: '\u{1F1EE}\u{1F1F3}',
  RU: '\u{1F1F7}\u{1F1FA}', TR: '\u{1F1F9}\u{1F1F7}',
};

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

function formatDuration(startISO: string): string {
  const diff = Date.now() - new Date(startISO).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m${sec % 60}s`;
  const hr = Math.floor(min / 60);
  return `${hr}h${min % 60}m`;
}

function isIP(host: string): boolean {
  return /^(\d{1,3}\.){3}\d{1,3}$/.test(host) || host.includes(':');
}

function guessRuleType(host: string): { type: string; payload: string } {
  if (!host) return { type: 'DOMAIN', payload: '' };
  if (isIP(host)) {
    return { type: host.includes(':') ? 'IP-CIDR6' : 'IP-CIDR', payload: host.includes('/') ? host : host.includes(':') ? `${host}/128` : `${host}/32` };
  }
  return { type: 'DOMAIN', payload: host };
}

export default function OverviewPage() {
  const t = useT();

  // mihomo status
  const [mihomoVersion, setMihomoVersion] = useState<string | null>(null);
  const [mihomoReachable, setMihomoReachable] = useState<boolean | null>(null);

  // connections
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loadingConns, setLoadingConns] = useState(false);

  // proxies with delay
  const [dbProxies, setDbProxies] = useState<Proxy[]>([]);
  const [delays, setDelays] = useState<Record<string, number | 'testing' | 'timeout' | 'error'>>({});
  const [testingAll, setTestingAll] = useState(false);

  // rule form for "add rule from connection"
  const [ruleFormOpen, setRuleFormOpen] = useState(false);
  const [ruleInitial, setRuleInitial] = useState<{ type: string; payload: string } | undefined>(undefined);

  const fetchConnections = useCallback(async () => {
    setLoadingConns(true);
    try {
      const res = await runtimeApi.connections();
      const data = res?.data;
      if (data && typeof data === 'object' && Array.isArray(data.connections)) {
        setConnections(data.connections);
      } else if (Array.isArray(data)) {
        setConnections(data);
      } else {
        setConnections([]);
      }
    } catch {
      setConnections([]);
    } finally {
      setLoadingConns(false);
    }
  }, []);

  const fetchProxies = useCallback(async () => {
    try {
      const res = await proxiesApi.list();
      setDbProxies(res.data ?? []);
    } catch {
      // non-fatal
    }
  }, []);

  useEffect(() => {
    runtimeApi
      .version()
      .then((res) => {
        const v = res?.data?.version ?? res?.data;
        setMihomoVersion(typeof v === 'string' ? v : 'unknown');
        setMihomoReachable(true);
      })
      .catch(() => setMihomoReachable(false));

    fetchConnections();
    fetchProxies();

    const interval = setInterval(fetchConnections, 5000);
    return () => clearInterval(interval);
  }, [fetchConnections, fetchProxies]);

  const handleCloseConnection = async (id: string) => {
    try {
      await runtimeApi.closeConnection(id);
      setConnections((prev) => prev.filter((c) => c.id !== id));
    } catch {
      toast.error(t('overview.closeConnFailed'));
    }
  };

  const handleAddRule = (host: string) => {
    const guess = guessRuleType(host);
    setRuleInitial(guess);
    setRuleFormOpen(true);
  };

  const handleRuleSave = async (data: Partial<Rule>) => {
    try {
      await rulesApi.create(data);
      toast.success(t('rules.created'));
      setRuleFormOpen(false);
    } catch {
      toast.error(t('rules.createFailed'));
    }
  };

  const testOneDelay = async (proxy: Proxy) => {
    setDelays((prev) => ({ ...prev, [proxy.id]: 'testing' }));
    try {
      const res = await runtimeApi.proxyDelay(proxy.name);
      const delay = res?.data?.delay;
      if (typeof delay === 'number' && delay > 0) {
        setDelays((prev) => ({ ...prev, [proxy.id]: delay }));
      } else {
        setDelays((prev) => ({ ...prev, [proxy.id]: 'timeout' }));
      }
    } catch {
      setDelays((prev) => ({ ...prev, [proxy.id]: 'error' }));
    }
  };

  const handleTestAll = async () => {
    setTestingAll(true);
    await Promise.all(dbProxies.map((proxy) => testOneDelay(proxy)));
    setTestingAll(false);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('overview.title')}</h1>
        <Badge variant={mihomoReachable ? 'default' : mihomoReachable === false ? 'destructive' : 'secondary'}>
          {mihomoReachable === null
            ? t('overview.checking')
            : mihomoReachable
              ? `mihomo ${mihomoVersion ?? ''}`
              : t('overview.unreachable')}
        </Badge>
      </div>

      {/* Active Connections */}
      <Card>
        <CardHeader className="border-b pb-3">
          <div className="flex items-center justify-between">
            <CardTitle>{t('overview.activeConnections')} ({connections.length})</CardTitle>
            <Button variant="ghost" size="icon-sm" onClick={fetchConnections} disabled={loadingConns}>
              <RefreshCwIcon className={`h-4 w-4 ${loadingConns ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          {connections.length === 0 ? (
            <p className="text-center py-8 text-sm text-muted-foreground">{t('overview.noConnections')}</p>
          ) : (
            <div className="max-h-[400px] overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('overview.host')}</TableHead>
                    <TableHead>{t('overview.network')}</TableHead>
                    <TableHead>{t('overview.chains')}</TableHead>
                    <TableHead>{t('overview.rule')}</TableHead>
                    <TableHead>{t('overview.traffic')}</TableHead>
                    <TableHead>{t('overview.duration')}</TableHead>
                    <TableHead className="w-16" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {connections.map((conn) => {
                    const host = conn.metadata?.host || conn.metadata?.destinationIP || '—';
                    const port = conn.metadata?.destinationPort;
                    const display = port ? `${host}:${port}` : host;
                    return (
                      <TableRow key={conn.id}>
                        <TableCell className="font-mono text-xs max-w-[200px] truncate" title={display}>
                          {display}
                        </TableCell>
                        <TableCell className="text-xs">{conn.metadata?.network ?? '—'}</TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-[150px] truncate">
                          {conn.chains?.join(' → ') ?? '—'}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-[150px] truncate">
                          {conn.rule ?? '—'}{conn.rulePayload ? ` (${conn.rulePayload})` : ''}
                        </TableCell>
                        <TableCell className="text-xs">
                          ↑{formatBytes(conn.upload ?? 0)} ↓{formatBytes(conn.download ?? 0)}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {conn.start ? formatDuration(conn.start) : '—'}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-0.5">
                            {conn.metadata?.host && (
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                onClick={() => handleAddRule(conn.metadata!.host!)}
                                title={t('runtime.addRule')}
                              >
                                <PlusIcon className="h-3 w-3" />
                              </Button>
                            )}
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => handleCloseConnection(conn.id)}
                              title={t('overview.closeConn')}
                            >
                              <XIcon className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Proxies with latency */}
      <Card>
        <CardHeader className="border-b pb-3">
          <div className="flex items-center justify-between">
            <CardTitle>{t('overview.proxies')} ({dbProxies.length})</CardTitle>
            <Button variant="outline" size="sm" onClick={handleTestAll} disabled={testingAll}>
              {testingAll ? (
                <>
                  <LoaderIcon className="h-3.5 w-3.5 animate-spin mr-1" />
                  {t('proxies.testing')}
                </>
              ) : (
                <>
                  <ActivityIcon className="h-3.5 w-3.5 mr-1" />
                  {t('proxies.testAllDelay')}
                </>
              )}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          {dbProxies.length === 0 ? (
            <p className="text-center py-8 text-sm text-muted-foreground">{t('proxies.noProxies')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('common.name')}</TableHead>
                  <TableHead>{t('proxies.country')}</TableHead>
                  <TableHead>{t('common.type')}</TableHead>
                  <TableHead>{t('proxies.delay')}</TableHead>
                  <TableHead className="w-8" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {dbProxies.map((proxy) => {
                  const delay = delays[proxy.id];
                  const flag = proxy.country ? (COUNTRY_FLAG[proxy.country] ?? '') : '';
                  return (
                    <TableRow key={proxy.id}>
                      <TableCell className="font-medium">{proxy.name}</TableCell>
                      <TableCell className="text-sm">
                        {flag ? `${flag} ${proxy.country}` : proxy.country || '—'}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{proxy.type}</Badge>
                      </TableCell>
                      <TableCell className="text-sm">
                        {delay === 'testing' ? (
                          <LoaderIcon className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
                        ) : delay === 'timeout' ? (
                          <span className="text-destructive">{t('proxies.timeout')}</span>
                        ) : delay === 'error' ? (
                          <span className="text-destructive">{t('proxies.error')}</span>
                        ) : typeof delay === 'number' ? (
                          <span className={delay < 300 ? 'text-green-600' : delay < 600 ? 'text-yellow-600' : 'text-red-600'}>
                            {delay}ms
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => testOneDelay(proxy)}
                          disabled={delay === 'testing'}
                          title={t('proxies.testDelay')}
                        >
                          <ActivityIcon className="h-3.5 w-3.5" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <RuleForm
        open={ruleFormOpen}
        onOpenChange={setRuleFormOpen}
        initialValues={ruleInitial}
        onSave={handleRuleSave}
      />
    </div>
  );
}
