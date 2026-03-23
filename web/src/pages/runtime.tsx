import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { RefreshCwIcon, ZapIcon, CheckIcon } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConnectionTable } from '@/components/runtime/connection-table';
import { LogViewer } from '@/components/runtime/log-viewer';
import { runtimeApi } from '@/api/runtime';

// ─── Proxies Tab ────────────────────────────────────────────────────────────

interface ProxyInfo {
  name: string;
  type: string;
  now?: string;
  all?: string[];
  history?: Array<{ time: string; delay: number }>;
  delay?: number;
}

function ProxiesTab() {
  const [proxies, setProxies] = useState<Record<string, ProxyInfo>>({});
  const [loading, setLoading] = useState(true);
  const [delays, setDelays] = useState<Record<string, number | null>>({});
  const [testingDelay, setTestingDelay] = useState<Record<string, boolean>>({});
  const [switching, setSwitching] = useState<Record<string, boolean>>({});

  const fetchProxies = useCallback(async () => {
    try {
      const res = await runtimeApi.proxies();
      const data = res?.data;
      if (data && typeof data === 'object' && data.proxies) {
        setProxies(data.proxies);
      } else {
        setProxies({});
      }
    } catch {
      setProxies({});
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProxies();
  }, [fetchProxies]);

  const handleTestDelay = async (name: string) => {
    setTestingDelay((prev) => ({ ...prev, [name]: true }));
    try {
      const res = await runtimeApi.proxyDelay(name);
      const delay = res?.data?.delay ?? res?.data;
      setDelays((prev) => ({ ...prev, [name]: typeof delay === 'number' ? delay : null }));
    } catch {
      setDelays((prev) => ({ ...prev, [name]: -1 }));
    } finally {
      setTestingDelay((prev) => ({ ...prev, [name]: false }));
    }
  };

  const handleSwitch = async (group: string, proxy: string) => {
    setSwitching((prev) => ({ ...prev, [`${group}:${proxy}`]: true }));
    try {
      await runtimeApi.switchProxy(group, proxy);
      setProxies((prev) => ({
        ...prev,
        [group]: { ...prev[group], now: proxy },
      }));
      toast.success(`Switched to ${proxy}`);
    } catch {
      toast.error('Failed to switch proxy');
    } finally {
      setSwitching((prev) => ({ ...prev, [`${group}:${proxy}`]: false }));
    }
  };

  function delayBadge(name: string, proxy: ProxyInfo) {
    const d = delays[name];
    const hist = proxy.history;
    const lastDelay = d !== undefined ? d : hist?.[hist.length - 1]?.delay;
    if (lastDelay == null) return null;
    if (lastDelay < 0) return <Badge variant="destructive">Timeout</Badge>;
    const color =
      lastDelay < 100
        ? 'text-green-600 dark:text-green-400'
        : lastDelay < 300
          ? 'text-yellow-600 dark:text-yellow-400'
          : 'text-red-500 dark:text-red-400';
    return <span className={`text-xs font-mono ${color}`}>{lastDelay}ms</span>;
  }

  if (loading) {
    return <div className="text-center py-12 text-muted-foreground">Loading proxies...</div>;
  }

  const groups = Object.values(proxies).filter(
    (p) => p.type === 'Selector' || p.type === 'URLTest' || p.type === 'Fallback' || p.type === 'LoadBalance' || p.type === 'Relay'
  );

  if (groups.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        No proxy groups found. Mihomo may not be running.
      </div>
    );
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {groups.map((group) => (
        <Card key={group.name}>
          <CardHeader className="border-b pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">{group.name}</CardTitle>
              <Badge variant="secondary">{group.type}</Badge>
            </div>
            {group.now && (
              <p className="text-xs text-muted-foreground mt-1">
                Selected: <span className="font-medium text-foreground">{group.now}</span>
              </p>
            )}
          </CardHeader>
          <CardContent className="pt-3">
            <div className="space-y-1.5">
              {(group.all ?? []).map((memberName) => {
                const member = proxies[memberName];
                const isSelected = group.now === memberName;
                const isSelectable = group.type === 'Selector';
                return (
                  <div
                    key={memberName}
                    className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      {isSelected && <CheckIcon className="h-3 w-3 text-green-500 shrink-0" />}
                      <span className="text-xs truncate">{memberName}</span>
                      {member && (
                        <span className="text-xs text-muted-foreground shrink-0">
                          ({member.type})
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      {delayBadge(memberName, member ?? { name: memberName, type: '' })}
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleTestDelay(memberName)}
                        disabled={testingDelay[memberName]}
                        title="Test delay"
                      >
                        <ZapIcon className="h-3 w-3" />
                      </Button>
                      {isSelectable && !isSelected && (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => handleSwitch(group.name, memberName)}
                          disabled={switching[`${group.name}:${memberName}`]}
                          title="Switch to this proxy"
                        >
                          <CheckIcon className="h-3 w-3" />
                        </Button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

// ─── Rules Tab ───────────────────────────────────────────────────────────────

interface RuntimeRule {
  type: string;
  payload: string;
  proxy: string;
}

function RulesTab() {
  const [rules, setRules] = useState<RuntimeRule[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    runtimeApi
      .rules()
      .then((res) => {
        const data = res?.data;
        if (Array.isArray(data)) setRules(data);
        else if (data?.rules) setRules(data.rules);
        else setRules([]);
      })
      .catch(() => setRules([]))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="text-center py-12 text-muted-foreground">Loading rules...</div>;

  if (rules.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        No rules found. Mihomo may not be running.
      </div>
    );
  }

  return (
    <div className="rounded-md border overflow-hidden">
      <table className="w-full text-sm">
        <thead className="bg-muted/50">
          <tr>
            <th className="px-4 py-2 text-left font-medium text-muted-foreground">Type</th>
            <th className="px-4 py-2 text-left font-medium text-muted-foreground">Payload</th>
            <th className="px-4 py-2 text-left font-medium text-muted-foreground">Proxy</th>
          </tr>
        </thead>
        <tbody>
          {rules.map((rule, i) => (
            <tr key={i} className="border-t hover:bg-muted/30">
              <td className="px-4 py-2">
                <Badge variant="outline" className="text-xs">
                  {rule.type}
                </Badge>
              </td>
              <td className="px-4 py-2 font-mono text-xs text-muted-foreground max-w-[280px] truncate">
                {rule.payload || '—'}
              </td>
              <td className="px-4 py-2 text-xs">{rule.proxy}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ─── Providers Tab ───────────────────────────────────────────────────────────

interface Provider {
  name: string;
  type: string;
  vehicleType: string;
  ruleCount?: number;
  updatedAt?: string;
}

function ProvidersTab() {
  const [providers, setProviders] = useState<Record<string, Provider>>({});
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState<Record<string, boolean>>({});

  const fetchProviders = useCallback(async () => {
    try {
      const res = await runtimeApi.providers();
      const data = res?.data;
      if (data && typeof data === 'object' && data.providers) {
        setProviders(data.providers);
      } else {
        setProviders({});
      }
    } catch {
      setProviders({});
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProviders();
  }, [fetchProviders]);

  const handleRefresh = async (name: string) => {
    setRefreshing((prev) => ({ ...prev, [name]: true }));
    try {
      await runtimeApi.refreshProvider(name);
      toast.success(`Provider "${name}" refreshed`);
      fetchProviders();
    } catch {
      toast.error(`Failed to refresh "${name}"`);
    } finally {
      setRefreshing((prev) => ({ ...prev, [name]: false }));
    }
  };

  if (loading) return <div className="text-center py-12 text-muted-foreground">Loading providers...</div>;

  const list = Object.values(providers);
  if (list.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        No providers found. Mihomo may not be running.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {list.map((provider) => (
        <Card key={provider.name}>
          <CardContent className="flex items-center justify-between py-4">
            <div className="space-y-1">
              <p className="font-medium text-sm">{provider.name}</p>
              <div className="flex items-center gap-2">
                <Badge variant="outline">{provider.type}</Badge>
                <Badge variant="secondary">{provider.vehicleType}</Badge>
                {provider.ruleCount != null && (
                  <span className="text-xs text-muted-foreground">{provider.ruleCount} rules</span>
                )}
              </div>
              {provider.updatedAt && (
                <p className="text-xs text-muted-foreground">
                  Updated: {new Date(provider.updatedAt).toLocaleString()}
                </p>
              )}
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleRefresh(provider.name)}
              disabled={refreshing[provider.name]}
            >
              <RefreshCwIcon className="h-4 w-4 mr-1.5" />
              Refresh
            </Button>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

// ─── Runtime Page ─────────────────────────────────────────────────────────────

export default function RuntimePage() {
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Runtime</h1>

      <Tabs defaultValue="connections">
        <TabsList>
          <TabsTrigger value="connections">Connections</TabsTrigger>
          <TabsTrigger value="proxies">Proxies</TabsTrigger>
          <TabsTrigger value="rules">Rules</TabsTrigger>
          <TabsTrigger value="providers">Providers</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="connections" className="mt-4">
          <ConnectionTable />
        </TabsContent>

        <TabsContent value="proxies" className="mt-4">
          <ProxiesTab />
        </TabsContent>

        <TabsContent value="rules" className="mt-4">
          <RulesTab />
        </TabsContent>

        <TabsContent value="providers" className="mt-4">
          <ProvidersTab />
        </TabsContent>

        <TabsContent value="logs" className="mt-4">
          <LogViewer logs={[]} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
