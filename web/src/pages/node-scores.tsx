import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import {
  EyeIcon,
  LoaderIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { NodeScoreDetail } from '@/components/node-scores/node-score-detail';
import { nodeScoresApi, toNodeScoreView, type NodeScore } from '@/api/node-scores';
import { readApiError } from '@/lib/api-error';
import { scoreBand } from '@/lib/node-score';
import { formatDateTime } from '@/lib/subscription-format';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore, useT } from '@/i18n';
import type { TranslationKey } from '@/i18n';

const BAND_KEY: Record<string, TranslationKey> = {
  high: 'nodeScores.scoreHigh',
  medium: 'nodeScores.scoreMedium',
  low: 'nodeScores.scoreLow',
  veryLow: 'nodeScores.scoreVeryLow',
};

const STATUS_KEY: Record<string, TranslationKey> = {
  pending: 'nodeScores.statusPending',
  checking: 'nodeScores.statusTesting',
  ok: 'nodeScores.statusOk',
  error: 'nodeScores.statusFailed',
  skipped: 'nodeScores.statusSkipped',
};

function statusVariant(status: string): 'default' | 'secondary' | 'outline' | 'destructive' {
  if (status === 'ok') return 'outline';
  if (status === 'error') return 'destructive';
  if (status === 'checking') return 'default';
  return 'secondary';
}

function scoreColor(score: number | null): string {
  const band = scoreBand(score);
  if (band === 'high') return 'text-emerald-700 dark:text-emerald-400';
  if (band === 'medium') return 'text-amber-700 dark:text-amber-400';
  if (band === 'low' || band === 'veryLow') return 'text-destructive';
  return 'text-muted-foreground';
}

function TypeBadges({ row }: { row: NodeScore }) {
  const t = useT();
  const flags: Array<[boolean | null, TranslationKey]> = [
    [row.is_residential, 'nodeScores.residential'],
    [row.is_datacenter, 'nodeScores.datacenter'],
    [row.is_vpn, 'nodeScores.vpn'],
    [row.is_proxy, 'nodeScores.proxy'],
    [row.is_tor, 'nodeScores.tor'],
  ];
  const shown = flags.filter(([value]) => value === true);
  if (shown.length === 0) {
    const allKnown = flags.every(([value]) => value !== null);
    return (
      <span className="text-xs text-muted-foreground">
        {allKnown ? t('nodeScores.noFlags') : t('common.unknown')}
      </span>
    );
  }
  return (
    <div className="flex flex-wrap gap-1">
      {shown.map(([, key]) => (
        <Badge
          key={key}
          variant={key === 'nodeScores.residential' ? 'default' : 'secondary'}
          className="h-5 px-1.5 text-[10px]"
        >
          {t(key)}
        </Badge>
      ))}
    </div>
  );
}

export default function NodeScoresPage() {
  const t = useT();
  const locale = useI18nStore((s) => s.locale) === 'zh' ? 'zh-CN' : 'en-GB';
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin');

  const [rows, setRows] = useState<NodeScore[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [search, setSearch] = useState('');
  const [kindFilter, setKindFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [refreshing, setRefreshing] = useState<Record<string, boolean>>({});
  const [batchBusy, setBatchBusy] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);

  const fetchScores = useCallback(async () => {
    try {
      const res = await nodeScoresApi.list();
      setRows(res.data ?? []);
      setLoadError(false);
    } catch {
      setLoadError(true);
      toast.error(t('nodeScores.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const hasActive = useMemo(
    () =>
      rows.some(
        (r) => r.status === 'pending' || r.status === 'checking' || r.status === 'testing'
      ),
    [rows]
  );

  useEffect(() => {
    fetchScores();
    const timer = setInterval(fetchScores, hasActive ? 10_000 : 60_000);
    return () => clearInterval(timer);
  }, [fetchScores, hasActive]);

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (kindFilter !== 'all' && row.node_kind !== kindFilter) return false;
      if (statusFilter !== 'all' && toNodeScoreView(row).status !== statusFilter) return false;
      if (!needle) return true;
      return [row.node_name, row.provider_name ?? '', row.exit_ip, row.country ?? '', row.asn_org ?? '', row.asn?.toString() ?? '']
        .join(' ')
        .toLowerCase()
        .includes(needle);
    });
  }, [rows, search, kindFilter, statusFilter]);

  const detailRow = detailId ? rows.find((r) => r.id === detailId) ?? null : null;

  const toggleAll = (checked: boolean) => {
    if (!checked) {
      setSelected(new Set());
      return;
    }
    setSelected(new Set(filtered.map((r) => r.id)));
  };

  const toggleOne = (id: string, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const refreshOne = async (id: string) => {
    if (!isAdmin) return;
    setRefreshing((prev) => ({ ...prev, [id]: true }));
    try {
      const res = await nodeScoresApi.refresh(id);
      toast.success(t('nodeScores.batchQueued', { count: res.data?.queued ?? 1 }));
      await fetchScores();
    } catch (err) {
      const info = await readApiError(err);
      toast.error(info.message || t('nodeScores.refreshFailed'));
    } finally {
      setRefreshing((prev) => ({ ...prev, [id]: false }));
    }
  };

  const refreshBatch = async (ids?: string[]) => {
    if (!isAdmin) return;
    setBatchBusy(true);
    try {
      const res = await nodeScoresApi.refreshBatch(ids);
      const queued = res.data?.queued ?? 0;
      toast.success(t('nodeScores.batchQueued', { count: queued }));
      setSelected(new Set());
      await fetchScores();
    } catch {
      toast.error(t('nodeScores.batchFailed'));
    } finally {
      setBatchBusy(false);
    }
  };

  const allFilteredSelected =
    filtered.length > 0 && filtered.every((row) => selected.has(row.id));

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="flex items-center gap-2 text-2xl font-bold">
          <ShieldCheckIcon className="h-6 w-6" />
          {t('nodeScores.title')}
        </h1>
        <div className="flex items-center gap-2">
          {selected.size > 0 && (
            <Button
              variant="outline"
              onClick={() => refreshBatch([...selected])}
              disabled={!isAdmin || batchBusy}
              title={isAdmin ? t('nodeScores.refreshSelected') : t('common.adminOnly')}
            >
              <RefreshCwIcon className={`mr-1 h-4 w-4 ${batchBusy ? 'animate-spin' : ''}`} />
              {t('nodeScores.refreshSelected')} ({selected.size})
            </Button>
          )}
          <Button
            variant="outline"
            onClick={() => refreshBatch()}
            disabled={!isAdmin || batchBusy || rows.length === 0}
            title={isAdmin ? t('nodeScores.refreshAll') : t('common.adminOnly')}
          >
            <RefreshCwIcon className={`mr-1 h-4 w-4 ${batchBusy ? 'animate-spin' : ''}`} />
            {batchBusy ? t('nodeScores.refreshingAll') : t('nodeScores.refreshAll')}
          </Button>
        </div>
      </div>

      <p className="text-sm text-muted-foreground">{t('nodeScores.disclaimer')}</p>
      <p className="text-xs text-muted-foreground">{t('nodeScores.cadenceHint')}</p>
      {!isAdmin && (
        <p className="text-xs text-muted-foreground">{t('nodeScores.readonlyHint')}</p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative max-w-xs flex-1">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('nodeScores.searchPlaceholder')}
            className="pl-8"
          />
        </div>
        <select
          className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm"
          value={kindFilter}
          onChange={(e) => setKindFilter(e.target.value)}
          aria-label={t('nodeScores.allKinds')}
        >
          <option value="all">{t('nodeScores.allKinds')}</option>
          <option value="static">{t('nodeScores.sourceStatic')}</option>
          <option value="provider">{t('nodeScores.sourceProvider')}</option>
        </select>
        <select
          className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          aria-label={t('nodeScores.allStatuses')}
        >
          <option value="all">{t('nodeScores.allStatuses')}</option>
          <option value="pending">{t('nodeScores.statusPending')}</option>
          <option value="checking">{t('nodeScores.statusTesting')}</option>
          <option value="ok">{t('nodeScores.statusOk')}</option>
          <option value="error">{t('nodeScores.statusFailed')}</option>
          <option value="skipped">{t('nodeScores.statusSkipped')}</option>
        </select>
        <span className="text-xs text-muted-foreground">
          {t('nodeScores.showingCount', { count: filtered.length })}
        </span>
      </div>

      {loading ? (
        <p className="text-muted-foreground">{t('common.loading')}</p>
      ) : loadError ? (
        <Card>
          <CardContent className="space-y-3 py-8 text-center">
            <p className="text-muted-foreground">{t('nodeScores.loadFailed')}</p>
            <Button variant="outline" onClick={fetchScores}>
              {t('common.refresh')}
            </Button>
          </CardContent>
        </Card>
      ) : filtered.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-muted-foreground">
            {t('nodeScores.empty')}
          </CardContent>
        </Card>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8">
                <input
                  type="checkbox"
                  aria-label={t('nodeScores.selectAll')}
                  checked={allFilteredSelected}
                  onChange={(e) => toggleAll(e.target.checked)}
                  className="h-4 w-4 rounded border border-input"
                />
              </TableHead>
              <TableHead>{t('common.name')}</TableHead>
              <TableHead>{t('nodeScores.exitIp')}</TableHead>
              <TableHead>{t('nodeScores.trustScore')}</TableHead>
              <TableHead>{t('nodeScores.ipType')}</TableHead>
              <TableHead>{t('nodeScores.country')} / {t('nodeScores.asn')}</TableHead>
              <TableHead>{t('nodeScores.kind')}</TableHead>
              <TableHead>{t('nodeScores.checkedAt')}</TableHead>
              <TableHead>{t('nodeScores.nextRefresh')}</TableHead>
              <TableHead className="text-right">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((row) => {
              const view = toNodeScoreView(row);
              const band = scoreBand(view.score);
              return (
                <TableRow
                  key={row.id}
                  className="cursor-pointer"
                  onClick={() => setDetailId(row.id)}
                >
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      aria-label={row.node_name}
                      checked={selected.has(row.id)}
                      onChange={(e) => toggleOne(row.id, e.target.checked)}
                      className="h-4 w-4 rounded border border-input"
                    />
                  </TableCell>
                  <TableCell className="max-w-[14rem]">
                    <div className="truncate font-medium">{row.node_name}</div>
                    <div className="truncate text-xs text-muted-foreground">
                      {view.kind === 'provider'
                        ? `${t('nodeScores.sourceProvider')} · ${view.providerName || t('common.unknown')}`
                        : t('nodeScores.sourceStatic')}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="font-mono text-xs">{view.exitIp ?? t('nodeScores.exitIpUnknown')}</div>
                    {view.exitIpCidr && (
                      <div className="font-mono text-[10px] text-muted-foreground">{view.exitIpCidr}</div>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5">
                      <span className={`text-lg font-semibold tabular-nums ${scoreColor(view.score)}`}>
                        {view.score ?? t('nodeScores.scoreMissing')}
                      </span>
                      {band && <Badge variant="secondary">{t(BAND_KEY[band])}</Badge>}
                    </div>
                    {view.rangeOnly && (
                      <Badge variant="outline" className="mt-0.5 border-amber-600/30 text-[10px] text-amber-700 dark:text-amber-400">
                        {t('nodeScores.rangeBadge')}
                      </Badge>
                    )}
                    {view.ipMismatch && (
                      <Badge variant="destructive" className="mt-0.5 text-[10px]">
                        {t('nodeScores.matchMismatch')}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <TypeBadges row={row} />
                  </TableCell>
                  <TableCell className="max-w-[12rem]">
                    <div className="truncate text-sm">
                      {[row.country, row.country_code ? `(${row.country_code})` : ''].join(' ').trim() || '—'}
                    </div>
                    <div className="truncate text-xs text-muted-foreground">
                      {row.asn ? `AS${row.asn}` : '—'}
                      {row.asn_org ? ` · ${row.asn_org}` : ''}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={statusVariant(row.status)}>
                      {t(STATUS_KEY[row.status] ?? 'nodeScores.statusPending')}
                    </Badge>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs tabular-nums">
                    {formatDateTime(view.checkedAt, locale) ?? t('nodeScores.neverChecked')}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs tabular-nums">
                    {formatDateTime(view.nextDue, locale) ?? t('nodeScores.nextDueUnknown')}
                  </TableCell>
                  <TableCell className="text-right" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        title={t('common.details')}
                        onClick={() => setDetailId(row.id)}
                      >
                        <EyeIcon className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        disabled={!isAdmin || !!refreshing[row.id]}
                        title={isAdmin ? t('nodeScores.refresh') : t('common.adminOnly')}
                        onClick={() => refreshOne(row.id)}
                      >
                        {refreshing[row.id] ? (
                          <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <RefreshCwIcon className="h-3.5 w-3.5" />
                        )}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}

      <p className="text-xs text-muted-foreground">{t('nodeScores.scopeNote')}</p>

      <NodeScoreDetail
        score={detailRow}
        open={!!detailRow}
        onOpenChange={(open) => {
          if (!open) setDetailId(null);
        }}
        isAdmin={isAdmin}
        refreshing={detailRow ? !!refreshing[detailRow.id] : false}
        onRefresh={refreshOne}
      />
    </div>
  );
}
