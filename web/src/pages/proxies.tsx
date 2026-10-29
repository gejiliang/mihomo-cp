import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Link } from 'react-router-dom';
import {
  PlusIcon,
  ActivityIcon,
  LoaderIcon,
  ShieldCheckIcon,
  AlertTriangleIcon,
  RefreshCwIcon,
} from 'lucide-react';
import { Button, buttonVariants } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ProxyList } from '@/components/proxies/proxy-list';
import { ProxyForm } from '@/components/proxies/proxy-form';
import type { Proxy } from '@/api/proxies';
import { proxiesApi } from '@/api/proxies';
import type { RuntimeProxyProvider } from '@/api/runtime';
import { runtimeApi } from '@/api/runtime';
import {
  ALL_SOURCES,
  STATIC_SOURCE,
  buildNodeRows,
  countryOptions,
  filterNodeRows,
  protocolLabel,
  protocolOptions,
  providerSource,
  providerSummaries,
} from '@/lib/node-inventory';
import { useT } from '@/i18n';

export default function ProxiesPage() {
  const t = useT();
  // Static (database) proxies: editable CRUD list.
  const [proxies, setProxies] = useState<Proxy[]>([]);
  // Native proxy-provider (subscription) nodes: read-only runtime inventory.
  const [providers, setProviders] = useState<RuntimeProxyProvider[]>([]);
  const [loading, setLoading] = useState(false);
  const [staticError, setStaticError] = useState(false);
  const [providersLoading, setProvidersLoading] = useState(false);
  const [providersError, setProvidersError] = useState(false);
  const [search, setSearch] = useState('');
  const [protocolFilter, setProtocolFilter] = useState('');
  const [sourceFilter, setSourceFilter] = useState(ALL_SOURCES);
  const [countryFilter, setCountryFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingProxy, setEditingProxy] = useState<Proxy | undefined>(undefined);
  const [testAllTrigger, setTestAllTrigger] = useState(0);
  const [testingAll, setTestingAll] = useState(false);

  const fetchProxies = useCallback(async () => {
    setLoading(true);
    try {
      // Filtering (static and subscription nodes) happens client-side so both
      // sources obey the same search/protocol/source filters.
      const res = await proxiesApi.list();
      setProxies(res.data ?? []);
      setStaticError(false);
    } catch {
      setStaticError(true);
      toast.error(t('proxies.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const fetchProviders = useCallback(async () => {
    setProvidersLoading(true);
    try {
      const res = await runtimeApi.proxyProviders();
      setProviders(res.data?.providers ?? []);
      setProvidersError(false);
    } catch {
      // Provider failure must not hide or block the static list.
      setProviders([]);
      setProvidersError(true);
    } finally {
      setProvidersLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProxies();
    fetchProviders();
    // Load once on mount; CRUD actions and the retry button refetch explicitly.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const allRows = useMemo(() => buildNodeRows(proxies, providers), [proxies, providers]);
  const rows = useMemo(
    () =>
      filterNodeRows(allRows, {
        search,
        protocol: protocolFilter,
        source: sourceFilter,
        country: countryFilter,
      }),
    [allRows, search, protocolFilter, sourceFilter, countryFilter]
  );

  const summaries = useMemo(() => providerSummaries(providers), [providers]);
  // Source values are internal (`__all__`, `__static__`, `provider:<name>`).
  // SelectValue gets the human label explicitly: Base UI only knows item labels
  // after the menu has been opened once, so the initial/collapsed trigger would
  // otherwise render the raw value.
  const providerPrefix = providerSource('');
  const selectedProviderName = sourceFilter.startsWith(providerPrefix)
    ? sourceFilter.slice(providerPrefix.length)
    : '';
  const selectedProvider = providers.find((p) => p.name === selectedProviderName);
  const sourceLabel = sourceFilter === STATIC_SOURCE
    ? t('proxies.sourceStatic')
    : selectedProviderName
      ? selectedProvider && selectedProvider.status !== 'ok'
        ? `${selectedProviderName} · ${t('proxies.unknown')}`
        : selectedProviderName
      : t('proxies.sourceAll');
  const hasFilters =
    search.trim() !== '' ||
    protocolFilter !== '' ||
    countryFilter !== '' ||
    sourceFilter !== ALL_SOURCES;
  const emptyMessage =
    selectedProvider && selectedProvider.status !== 'ok'
      ? t('proxies.providerUnknown')
      : hasFilters
        ? t('proxies.noMatchingNodes')
        : staticError
          ? t('proxies.loadFailed')
          : t('proxies.noProxies');

  const handleAddClick = () => {
    setEditingProxy(undefined);
    setFormOpen(true);
  };

  const handleEditClick = (proxy: Proxy) => {
    setEditingProxy(proxy);
    setFormOpen(true);
  };

  const handleSave = async (data: { name: string; type: string; config: Record<string, unknown> }) => {
    try {
      if (editingProxy) {
        await proxiesApi.update(editingProxy.id, { name: data.name, type: data.type, config: data.config });
        toast.success(t('proxies.updated'));
      } else {
        await proxiesApi.create({ name: data.name, type: data.type, config: data.config });
        toast.success(t('proxies.created'));
      }
      setFormOpen(false);
      setEditingProxy(undefined);
      fetchProxies();
    } catch {
      toast.error(editingProxy ? t('proxies.updateFailed') : t('proxies.createFailed'));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('proxies.title')}</h1>
        <div className="flex items-center gap-2">
          <Link to="/node-scores" className={buttonVariants({ variant: 'outline' })}>
            <ShieldCheckIcon className="h-3.5 w-3.5" />
            {t('proxies.ipScores')}
          </Link>
          <Button
            variant="outline"
            onClick={() => { setTestingAll(true); setTestAllTrigger((n) => n + 1); }}
            disabled={testingAll}
          >
            {testingAll ? (
              <>
                <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
                {t('proxies.testing')}
              </>
            ) : (
              <>
                <ActivityIcon className="h-3.5 w-3.5" />
                {t('proxies.testAllDelay')}
              </>
            )}
          </Button>
          <Button onClick={handleAddClick}>
            <PlusIcon />
            {t('proxies.add')}
          </Button>
        </div>
      </div>

      {providersError && (
        <Alert variant="destructive">
          <AlertTriangleIcon />
          <AlertTitle>{t('proxies.providerLoadFailed')}</AlertTitle>
          <AlertDescription>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span>{t('proxies.providerLoadFailedDesc')}</span>
              <Button variant="outline" size="sm" onClick={fetchProviders} disabled={providersLoading}>
                {providersLoading ? (
                  <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCwIcon className="h-3.5 w-3.5" />
                )}
                {t('proxies.retry')}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}

      <div className="flex items-center gap-2 flex-wrap">
        <Input
          type="text"
          placeholder={t('proxies.searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
        <Select
          value={sourceFilter}
          onValueChange={(v) => setSourceFilter(v || ALL_SOURCES)}
        >
          <SelectTrigger className="w-48">
            <SelectValue placeholder={t('proxies.sourceAll')}>{sourceLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_SOURCES}>{t('proxies.sourceAll')}</SelectItem>
            <SelectItem value={STATIC_SOURCE}>{t('proxies.sourceStatic')}</SelectItem>
            {providers.map((p) => (
              <SelectItem key={p.name} value={providerSource(p.name)}>
                {p.status === 'ok' ? p.name : `${p.name} · ${t('proxies.unknown')}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={protocolFilter}
          onValueChange={(v) => setProtocolFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder={t('common.allTypes')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('common.allTypes')}</SelectItem>
            {protocolOptions(allRows).map((pt) => (
              <SelectItem key={pt} value={pt}>
                {protocolLabel(pt)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={countryFilter}
          onValueChange={(v) => setCountryFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder={t('proxies.allCountries')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('proxies.allCountries')}</SelectItem>
            {countryOptions(allRows).map((c) => (
              <SelectItem key={c} value={c}>
                {c}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {summaries.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {summaries.map((s) => (
            <Badge
              key={s.name}
              variant={s.status === 'ok' ? 'secondary' : 'destructive'}
              title={s.status === 'ok' ? t('proxies.providerStatusOK') : t('proxies.providerUnknown')}
            >
              {s.name}: {s.nodeCount === null ? t('proxies.unknown') : s.nodeCount}
            </Badge>
          ))}
        </div>
      )}

      {loading && allRows.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">{t('common.loading')}</div>
      ) : (
        <ProxyList
          rows={rows}
          providers={providers}
          onEdit={handleEditClick}
          onDeleted={fetchProxies}
          onCopied={fetchProxies}
          testAllTrigger={testAllTrigger}
          onTestAllDone={() => setTestingAll(false)}
          emptyMessage={emptyMessage}
        />
      )}

      <ProxyForm
        open={formOpen}
        onOpenChange={(o) => {
          setFormOpen(o);
          if (!o) setEditingProxy(undefined);
        }}
        proxy={editingProxy}
        onSave={handleSave}
      />
    </div>
  );
}
