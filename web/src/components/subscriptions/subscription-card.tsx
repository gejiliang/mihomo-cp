import { Link } from 'react-router-dom';
import {
  AlertTriangleIcon,
  CalendarClockIcon,
  ExternalLinkIcon,
  LinkIcon,
  LoaderIcon,
  PencilIcon,
  RefreshCwIcon,
  RssIcon,
  TimerIcon,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { Subscription } from '@/api/subscriptions';
import {
  daysUntil,
  displayMaskedUrl,
  formatApiDateTime,
  formatBytesExact,
  formatDateTime,
  formatPercent,
} from '@/lib/subscription-format';
import { useI18nStore, useT } from '@/i18n';
import type { TranslationKey } from '@/i18n';

const STATUS_KEY: Record<Subscription['status'], TranslationKey> = {
  ok: 'subscriptions.statusOk',
  stale: 'subscriptions.statusStale',
  error: 'subscriptions.statusError',
  disabled: 'subscriptions.statusDisabled',
};

const SOURCE_KEY: Record<string, TranslationKey> = {
  vendor: 'subscriptions.sourceVendor',
  local: 'subscriptions.sourceLocal',
  manual: 'subscriptions.sourceManual',
  none: 'subscriptions.sourceNone',
};

const EXPIRY_SOURCE_KEY: Record<string, TranslationKey> = {
  vendor: 'subscriptions.expirySourceVendor',
  manual: 'subscriptions.expirySourceManual',
  unknown: 'subscriptions.expirySourceUnknown',
};

function statusClass(status: Subscription['status']): string {
  switch (status) {
    case 'ok':
      return 'border-emerald-600/30 text-emerald-700 dark:text-emerald-400';
    case 'stale':
      return 'border-amber-600/30 text-amber-700 dark:text-amber-400';
    case 'error':
      return 'border-destructive/30 text-destructive';
    default:
      return 'text-muted-foreground';
  }
}

function meterColor(percent: number | null): string {
  if (percent === null) return '#898781';
  if (percent >= 95) return '#d03b3b';
  if (percent >= 80) return '#fab219';
  return '#2a78d6';
}

export function SubscriptionCard({
  subscription,
  refreshing,
  isAdmin,
  onRefresh,
  onEdit,
}: {
  subscription: Subscription;
  refreshing: boolean;
  isAdmin: boolean;
  onRefresh: (sub: Subscription) => void;
  onEdit: (sub: Subscription) => void;
}) {
  const t = useT();
  const locale = useI18nStore((s) => s.locale) === 'zh' ? 'zh-CN' : 'en-GB';
  const s = subscription;

  const percent = formatPercent(s.used_bytes, s.quota_bytes);
  const neverFetched = !s.userinfo_fetched_at && !s.last_success_at;
  const statusKey =
    s.status === 'stale' && !s.last_success_at
      ? 'subscriptions.statusPending'
      : STATUS_KEY[s.status];
  const maskedUrl = displayMaskedUrl(s.url_masked, null);
  const expires = formatApiDateTime(s.expires_at, locale, s.timezone);
  const nextReset = formatApiDateTime(s.next_reset, locale, s.timezone);
  const resetIn = daysUntil(s.next_reset);
  const usageUpdated = formatDateTime(s.userinfo_fetched_at, locale);
  const lastSuccess = formatDateTime(s.last_success_at, locale);
  const lastRefresh = formatDateTime(s.last_refresh_at, locale);
  const cycleEnd = formatApiDateTime(s.cycle_end, locale, s.timezone);
  const timezone = s.timezone || t('subscriptions.serverTimezone');
  const schedule = [
    `${t('subscriptions.resetDay')} ${s.reset_day || t('common.unknown')}`,
    `${String(s.reset_hour).padStart(2, '0')}:${String(s.reset_minute).padStart(2, '0')}`,
    timezone,
  ].join(' · ');

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex flex-wrap items-center gap-2 text-base">
              <RssIcon className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="truncate">{s.name}</span>
              <Badge variant="outline" className={statusClass(s.status)}>
                {t(statusKey)}
              </Badge>
            </CardTitle>
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1">
                <LinkIcon className="h-3 w-3" />
                {s.provider_name || t('subscriptions.providerNone')}
              </span>
              {s.provider_source === 'created' && (
                <Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
                  {t('subscriptions.providerSourceCreated')}
                </Badge>
              )}
              {s.provider_source === 'linked' && (
                <Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
                  {t('subscriptions.providerSourceLinked')}
                </Badge>
              )}
              <span>
                {t('subscriptions.nodeCount')}:{' '}
                {s.node_count === null ? t('common.unknown') : s.node_count}
              </span>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={!isAdmin || refreshing || !s.enabled}
              title={isAdmin ? t('subscriptions.refresh') : t('common.adminOnly')}
              onClick={() => onRefresh(s)}
            >
              {refreshing ? (
                <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCwIcon className="h-3.5 w-3.5" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={!isAdmin}
              title={isAdmin ? t('common.edit') : t('common.adminOnly')}
              onClick={() => onEdit(s)}
            >
              <PencilIcon className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        {maskedUrl && (
          <p className="mt-1 truncate font-mono text-xs text-muted-foreground" title={t('subscriptions.urlMaskedHint')}>
            {maskedUrl}
          </p>
        )}
        {!s.provider_exists && s.provider_name && (
          <p className="mt-1 flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
            <AlertTriangleIcon className="h-3 w-3" />
            {t('subscriptions.providerMissing')}
          </p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <div>
          <div className="flex items-baseline justify-between gap-3">
            <span className="text-sm text-muted-foreground">{t('subscriptions.remaining')}</span>
            <span className="text-lg font-semibold tabular-nums">
              {formatBytesExact(s.remaining_bytes) ?? t('common.unknown')}
            </span>
          </div>
          <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
            {percent !== null && (
              <div
                className="h-full rounded-full transition-all"
                style={{ width: `${percent}%`, backgroundColor: meterColor(percent) }}
              />
            )}
          </div>
          <div className="mt-1.5 flex flex-wrap justify-between gap-2 text-xs text-muted-foreground tabular-nums">
            <span>
              {t('subscriptions.used')}: {formatBytesExact(s.used_bytes) ?? t('common.unknown')}
            </span>
            <span>
              {t('subscriptions.total')}: {formatBytesExact(s.quota_bytes) ?? t('common.unknown')}
            </span>
          </div>
          {s.local_used_bytes !== null && (
            <div className="mt-1 flex justify-between text-xs text-muted-foreground tabular-nums">
              <span>{t('subscriptions.localUsed')}</span>
              <span>{formatBytesExact(s.local_used_bytes) ?? t('common.unknown')}</span>
            </div>
          )}
        </div>

        <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <CalendarClockIcon className="h-3.5 w-3.5" />
            {t('subscriptions.expiresAt')}
          </span>
          <span className="text-right">
            {expires ?? <span className="text-muted-foreground">{t('subscriptions.expiryUnknown')}</span>}
            <Badge variant="secondary" className="ml-2 h-4 px-1.5 text-[10px]">
              {t(EXPIRY_SOURCE_KEY[s.expires_source] ?? 'subscriptions.expirySourceUnknown')}
            </Badge>
          </span>

          <span className="flex items-center gap-1.5 text-muted-foreground">
            <TimerIcon className="h-3.5 w-3.5" />
            {t('subscriptions.nextReset')}
          </span>
          <span className="text-right">
            {nextReset ?? t('common.unknown')}
            {resetIn !== null && (
              <span className="ml-2 text-xs text-muted-foreground">
                {t('subscriptions.resetInDays', { days: resetIn })}
              </span>
            )}
            <span className="block text-xs text-muted-foreground">{schedule}</span>
          </span>

          <span className="text-muted-foreground">{t('subscriptions.usageSource')}</span>
          <span className="text-right">{t(SOURCE_KEY[s.usage_source] ?? 'subscriptions.sourceNone')}</span>

          <span className="text-muted-foreground">{t('subscriptions.dataUpdatedAt')}</span>
          <span className="text-right tabular-nums">{usageUpdated ?? t('subscriptions.neverRefreshed')}</span>

          <span className="text-muted-foreground">{t('subscriptions.lastSuccess')}</span>
          <span className="text-right tabular-nums">{lastSuccess ?? t('subscriptions.neverRefreshed')}</span>

          <span className="text-muted-foreground">{t('subscriptions.lastRefresh')}</span>
          <span className="text-right tabular-nums">{lastRefresh ?? t('subscriptions.neverRefreshed')}</span>
        </div>

        {cycleEnd && (
          <p className="text-xs text-muted-foreground">
            {t('subscriptions.cycleEnds', { time: cycleEnd })}
          </p>
        )}

        {s.stale && s.enabled && (
          <p className="rounded-md border border-amber-600/30 bg-amber-500/5 px-2.5 py-1.5 text-xs text-amber-700 dark:text-amber-400">
            {neverFetched ? t('subscriptions.neverFetchedHint') : t('subscriptions.staleHint')}
          </p>
        )}

        {s.last_error && (
          <p className="flex items-start gap-1.5 rounded-md border border-destructive/30 bg-destructive/5 px-2.5 py-1.5 text-xs text-destructive">
            <AlertTriangleIcon className="mt-0.5 h-3 w-3 shrink-0" />
            <span className="break-words">
              {t('subscriptions.lastError')}: {s.last_error}
            </span>
          </p>
        )}

        {s.draft_changed && (
          <Link
            to="/publish"
            className="flex items-center gap-1.5 text-xs font-medium text-primary hover:underline"
          >
            {t('subscriptions.draftPendingHint')}
            <ExternalLinkIcon className="h-3 w-3" />
          </Link>
        )}
      </CardContent>
    </Card>
  );
}

export { STATUS_KEY, SOURCE_KEY, EXPIRY_SOURCE_KEY };
