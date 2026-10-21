import {
  AlertTriangleIcon,
  ArrowUpRightIcon,
  ExternalLinkIcon,
  GlobeIcon,
  LoaderIcon,
  RefreshCwIcon,
  ServerIcon,
  ShieldCheckIcon,
  NetworkIcon,
} from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Separator } from '@/components/ui/separator';
import { toNodeScoreView, type NodeScore, type NodeScoreView } from '@/api/node-scores';
import type { IpIntel, SourceIntel } from '@/lib/node-score';
import { scoreBand } from '@/lib/node-score';
import { formatApiDateTime } from '@/lib/subscription-format';
import { useI18nStore, useT } from '@/i18n';
import type { TranslationKey } from '@/i18n';

const BAND_KEY: Record<string, TranslationKey> = {
  high: 'nodeScores.scoreHigh',
  medium: 'nodeScores.scoreMedium',
  low: 'nodeScores.scoreLow',
  veryLow: 'nodeScores.scoreVeryLow',
};

const MATCH_KEY: Record<string, TranslationKey> = {
  exact: 'nodeScores.matchExact',
  cidr: 'nodeScores.matchCidr',
  mismatch: 'nodeScores.matchMismatch',
  unknown: 'nodeScores.matchUnknown',
};

const STATUS_KEY: Record<string, TranslationKey> = {
  pending: 'nodeScores.statusPending',
  checking: 'nodeScores.statusTesting',
  ok: 'nodeScores.statusOk',
  error: 'nodeScores.statusFailed',
  skipped: 'nodeScores.statusSkipped',
};

const EXIT_SOURCE_KEY: Record<string, TranslationKey> = {
  'claude-trace': 'nodeScores.exitSourceClaude',
  'cloudflare-trace': 'nodeScores.exitSourceCloudflare',
};

function matchClass(match: string): string {
  if (match === 'exact') return 'border-emerald-600/30 text-emerald-700 dark:text-emerald-400';
  if (match === 'cidr') return 'border-amber-600/30 text-amber-700 dark:text-amber-400';
  if (match === 'mismatch') return 'border-destructive/30 text-destructive';
  return 'text-muted-foreground';
}

function scoreClass(score: number | null, band: string | null): string {
  if (score === null) return 'text-muted-foreground';
  if (band === 'high') return 'text-emerald-700 dark:text-emerald-400';
  if (band === 'medium') return 'text-amber-700 dark:text-amber-400';
  return 'text-destructive';
}

function boolText(value: boolean | null, t: ReturnType<typeof useT>): string {
  if (value === null) return t('common.unknown');
  return value ? t('common.yes') : t('common.no');
}

function fact(value: string | number | null): string {
  return value === null || value === '' ? '—' : String(value);
}

function IntelFacts({ intel }: { intel: IpIntel }) {
  const t = useT();
  return (
    <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
      <span className="text-muted-foreground">{t('nodeScores.country')}</span>
      <span className="text-right">
        {fact(intel.country)}
        {intel.countryCode ? ` (${intel.countryCode})` : ''}
      </span>
      <span className="text-muted-foreground">{t('nodeScores.asn')}</span>
      <span className="text-right tabular-nums">{fact(intel.asn)}</span>
      <span className="text-muted-foreground">{t('nodeScores.asnOrg')}</span>
      <span className="truncate text-right" title={intel.asnOrg ?? undefined}>
        {fact(intel.asnOrg)}
      </span>
      <span className="text-muted-foreground">{t('nodeScores.companyType')}</span>
      <span className="text-right">{fact(intel.companyType)}</span>
      <span className="text-muted-foreground">{t('nodeScores.residential')}</span>
      <span className="text-right">{boolText(intel.isResidential, t)}</span>
      <span className="text-muted-foreground">{t('nodeScores.datacenter')}</span>
      <span className="text-right">{boolText(intel.isDatacenter, t)}</span>
      <span className="text-muted-foreground">{t('nodeScores.vpn')}</span>
      <span className="text-right">{boolText(intel.isVpn, t)}</span>
      <span className="text-muted-foreground">{t('nodeScores.proxy')}</span>
      <span className="text-right">{boolText(intel.isProxy, t)}</span>
      <span className="text-muted-foreground">{t('nodeScores.tor')}</span>
      <span className="text-right">{boolText(intel.isTor, t)}</span>
      {intel.isMobile !== null && (
        <>
          <span className="text-muted-foreground">{t('nodeScores.mobile')}</span>
          <span className="text-right">{boolText(intel.isMobile, t)}</span>
        </>
      )}
      {intel.isCrawler !== null && (
        <>
          <span className="text-muted-foreground">{t('nodeScores.crawler')}</span>
          <span className="text-right">{boolText(intel.isCrawler, t)}</span>
        </>
      )}
      {intel.abuserScore && (
        <>
          <span className="text-muted-foreground">{t('nodeScores.abuserScore')}</span>
          <span className="text-right">{intel.abuserScore}</span>
        </>
      )}
    </div>
  );
}

function SourceHeader({
  title,
  score,
  match,
  sourceUrl,
}: {
  title: string;
  score: number | null;
  match: string;
  sourceUrl?: string;
}) {
  const t = useT();
  const band = scoreBand(score);
  return (
    <div className="flex items-start justify-between gap-2">
      <div>
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{t('nodeScores.trustScore')}</p>
      </div>
      <div className="flex items-center gap-2">
        <span className={`text-2xl font-semibold tabular-nums ${scoreClass(score, band)}`}>
          {score === null ? t('nodeScores.scoreMissing') : score}
        </span>
        {band && <Badge variant="secondary">{t(BAND_KEY[band])}</Badge>}
        {sourceUrl && (
          <a
            href={sourceUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="text-muted-foreground hover:text-foreground"
            title={t('nodeScores.sourceLink')}
          >
            <ArrowUpRightIcon className="h-4 w-4" />
          </a>
        )}
      </div>
      <Badge variant="outline" className={`ml-2 ${matchClass(match)}`}>
        {t(MATCH_KEY[match] ?? 'nodeScores.matchUnknown')}
      </Badge>
    </div>
  );
}

function ClaudeSource({ view }: { view: NodeScoreView }) {
  const t = useT();
  const source = view.sources.find((s) => s.source === 'claude');
  const score = view.score ?? source?.trustScore ?? null;
  const match = view.ipMatch;
  const intel = source?.intel;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <SourceHeader title={t('nodeScores.claudeSource')} score={score} match={match} />
      <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <span className="text-muted-foreground">{t('nodeScores.ipMatch')}</span>
        <span className="text-right">
          <span className="font-mono text-xs">{fact(view.queriedIp)}</span>
          {' → '}
          <span className="font-mono text-xs">{fact(view.matchedIp ?? source?.responseIp ?? null)}</span>
        </span>
        {source?.cidr && (
          <>
            <span className="text-muted-foreground">CIDR</span>
            <span className="text-right font-mono text-xs">{source.cidr}</span>
          </>
        )}
      </div>
      {view.ipMismatch && (
        <p className="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5 text-xs text-destructive">
          {t('nodeScores.ipMismatchHint')}
        </p>
      )}
      {!view.ipMismatch && view.rangeOnly && (
        <p className="rounded-md border border-amber-600/30 bg-amber-500/5 px-2 py-1.5 text-xs text-amber-700 dark:text-amber-400">
          {t('nodeScores.cidrHint')}
        </p>
      )}
      {intel ? (
        <>
          <IntelFacts intel={intel} />
          {intel.sources.length > 0 && (
            <p className="text-xs text-muted-foreground">
              {t('nodeScores.sources')}: {intel.sources.join(', ')}
            </p>
          )}
        </>
      ) : (
        <p className="text-xs text-muted-foreground">{t('nodeScores.sourceIdle')}</p>
      )}
    </div>
  );
}

function LookupSource({ view }: { view: NodeScoreView }) {
  const t = useT();
  const source: SourceIntel | undefined = view.sources.find((s) => s.source === 'lookup');
  if (!source) {
    return (
      <div className="space-y-3 rounded-lg border p-3">
        <SourceHeader
          title={t('nodeScores.lookupSource')}
          score={null}
          match="unknown"
        />
        <p className="text-xs text-muted-foreground">{t('nodeScores.lookupNotStored')}</p>
      </div>
    );
  }
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <SourceHeader
        title={t('nodeScores.lookupSource')}
        score={source.trustScore}
        match={source.ipMatch}
      />
      <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <span className="text-muted-foreground">{t('nodeScores.ipMatch')}</span>
        <span className="text-right font-mono text-xs">
          {fact(source.queryIp)} → {fact(source.responseIp)}
        </span>
        {source.cidr && (
          <>
            <span className="text-muted-foreground">CIDR</span>
            <span className="text-right font-mono text-xs">{source.cidr}</span>
          </>
        )}
      </div>
      {source.ipMatch === 'mismatch' && (
        <p className="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5 text-xs text-destructive">
          {t('nodeScores.ipMismatchHint')}
        </p>
      )}
      {source.ipMatch === 'cidr' && (
        <p className="rounded-md border border-amber-600/30 bg-amber-500/5 px-2 py-1.5 text-xs text-amber-700 dark:text-amber-400">
          {t('nodeScores.cidrHint')}
        </p>
      )}
      {source.intel.aiVerdictLabel && (
        <div className="rounded-md bg-muted/50 px-2 py-1.5 text-xs">
          <p className="font-medium">{source.intel.aiVerdictLabel}</p>
          {source.intel.aiVerdictReason && (
            <p className="text-muted-foreground">{source.intel.aiVerdictReason}</p>
          )}
        </div>
      )}
      <IntelFacts intel={source.intel} />
      {source.intel.sources.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('nodeScores.sources')}: {source.intel.sources.join(', ')}
        </p>
      )}
    </div>
  );
}

export function NodeScoreDetail({
  score,
  open,
  onOpenChange,
  isAdmin,
  refreshing,
  onRefresh,
}: {
  score: NodeScore | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isAdmin: boolean;
  refreshing: boolean;
  onRefresh: (id: string) => void;
}) {
  const t = useT();
  const locale = useI18nStore((s) => s.locale) === 'zh' ? 'zh-CN' : 'en-GB';
  if (!score) return null;
  const view = toNodeScoreView(score);
  const detailUrl =
    score.detail_url ||
    (view.exitIp ? `https://ip.net.coffee/ip/${encodeURIComponent(view.exitIp)}` : '');
  const scoreUrl = score.score_url || 'https://ip.net.coffee/claude/';

  return (
    <Dialog open={open} onOpenChange={(o) => onOpenChange(o)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-2">
            <ShieldCheckIcon className="h-5 w-5 text-muted-foreground" />
            {t('nodeScores.detailTitle')}
            <Badge variant="outline" className={matchClass(view.ipMatch)}>
              {t(MATCH_KEY[view.ipMatch] ?? 'nodeScores.matchUnknown')}
            </Badge>
            <Badge variant="secondary">{t(STATUS_KEY[view.status] ?? 'nodeScores.statusPending')}</Badge>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <Alert>
            <AlertTriangleIcon />
            <AlertTitle>{t('nodeScores.trustScore')}</AlertTitle>
            <AlertDescription>{t('nodeScores.disclaimer')}</AlertDescription>
          </Alert>

          <section className="space-y-2">
            <h3 className="text-sm font-medium text-muted-foreground">{t('nodeScores.detailIdentity')}</h3>
            <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <span className="text-muted-foreground">{t('common.name')}</span>
              <span className="truncate text-right font-medium">{view.nodeName}</span>
              <span className="text-muted-foreground">{t('nodeScores.sourceProvider')}</span>
              <span className="text-right">
                {view.kind === 'provider'
                  ? view.providerName || t('common.unknown')
                  : t('nodeScores.sourceStatic')}
              </span>
              <span className="text-muted-foreground">{t('nodeScores.sources')}</span>
              <span className="text-right text-xs">
                {score.sources && score.sources.length > 0
                  ? score.sources.join(', ')
                  : t('common.unknown')}
              </span>
            </div>
          </section>

          <section className="space-y-2">
            <h3 className="flex items-center gap-1.5 text-sm font-medium text-muted-foreground">
              <NetworkIcon className="h-4 w-4" />
              {t('nodeScores.detailEgress')}
            </h3>
            <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <span className="text-muted-foreground">{t('nodeScores.exitIp')}</span>
              <span className="text-right font-mono">
                {view.exitIp ?? t('nodeScores.exitIpUnknown')}
                {view.exitIpCidr && (
                  <span className="ml-2 text-xs text-muted-foreground">({view.exitIpCidr})</span>
                )}
              </span>
              <span className="text-muted-foreground">{t('nodeScores.ipMatch')}</span>
              <span className="text-right">{t(MATCH_KEY[view.ipMatch] ?? 'nodeScores.matchUnknown')}</span>
              <span className="text-muted-foreground">{t('nodeScores.exitSource')}</span>
              <span className="text-right">
                {score.exit_ip_source
                  ? EXIT_SOURCE_KEY[score.exit_ip_source]
                    ? t(EXIT_SOURCE_KEY[score.exit_ip_source])
                    : score.exit_ip_source
                  : t('common.unknown')}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">{t('nodeScores.scopeNote')}</p>
          </section>

          <Separator />

          <div className="grid gap-3 lg:grid-cols-2">
            <ClaudeSource view={view} />
            <LookupSource view={view} />
          </div>

          <Separator />

          <section className="space-y-2">
            <h3 className="text-sm font-medium text-muted-foreground">{t('nodeScores.detailTiming')}</h3>
            <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <span className="text-muted-foreground">{t('nodeScores.checkedAt')}</span>
              <span className="text-right tabular-nums">
                {formatApiDateTime(view.checkedAt, locale) ?? t('nodeScores.neverChecked')}
              </span>
              <span className="text-muted-foreground">{t('nodeScores.lastSuccess')}</span>
              <span className="text-right tabular-nums">
                {formatApiDateTime(view.lastSuccessAt, locale) ?? t('common.never')}
              </span>
              <span className="text-muted-foreground">{t('nodeScores.nextRefresh')}</span>
              <span className="text-right tabular-nums">
                {formatApiDateTime(view.nextDue, locale) ?? t('nodeScores.nextDueUnknown')}
              </span>
            </div>
          </section>

          {view.error && (
            <Alert variant="destructive">
              <AlertTriangleIcon />
              <AlertTitle>{t('nodeScores.detailError')}</AlertTitle>
              <AlertDescription>
                <span className="break-words">{view.error}</span>
                {view.errorCode && (
                  <span className="mt-1 block text-xs">
                    {t('nodeScores.errorCode')}: {view.errorCode}
                  </span>
                )}
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-3 text-xs">
            {scoreUrl && (
              <a
                href={scoreUrl}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-primary hover:underline"
              >
                <ShieldCheckIcon className="h-3 w-3" />
                {t('nodeScores.claudeSource')}
                <ExternalLinkIcon className="h-3 w-3" />
              </a>
            )}
            {detailUrl && (
              <a
                href={detailUrl}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-primary hover:underline"
              >
                <GlobeIcon className="h-3 w-3" />
                {t('nodeScores.lookupSource')}
                <ExternalLinkIcon className="h-3 w-3" />
              </a>
            )}
          </div>
        </div>

        <DialogFooter className="flex items-center justify-between sm:justify-between">
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <ServerIcon className="h-3 w-3" />
            {t('nodeScores.probeMethod')}: {t('nodeScores.probeMethodValue')}
          </span>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              onClick={() => onRefresh(score.id)}
              disabled={!isAdmin || refreshing}
              title={isAdmin ? t('nodeScores.refresh') : t('common.adminOnly')}
            >
              {refreshing ? (
                <LoaderIcon className="mr-1 h-4 w-4 animate-spin" />
              ) : (
                <RefreshCwIcon className="mr-1 h-4 w-4" />
              )}
              {refreshing ? t('nodeScores.refreshing') : t('nodeScores.refresh')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
