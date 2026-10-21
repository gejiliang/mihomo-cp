import { api } from './client';
import {
  normalizeIpInfo,
  normalizeIpMatch,
  parseScoreSources,
  type IpIntel,
  type IpMatch,
  type SourceIntel,
} from '@/lib/node-score';
import { normalizeTimestamp } from './subscriptions';

export interface NodeScore {
  id: string;
  node_name: string;
  node_kind: 'static' | 'provider' | string;
  provider_name: string | null;
  status: 'pending' | 'checking' | 'ok' | 'error' | 'skipped' | 'testing' | 'failed' | string;
  error: string;
  error_code: string;
  exit_ip: string;
  exit_ip_cidr: string | null;
  exit_ip_source: string;
  trust_score: number | null;
  score_source: string;
  score_url: string;
  detail_url: string;
  queried_ip: string | null;
  matched_ip: string | null;
  ip_mismatch: boolean;
  range_only: boolean;
  claude_ip_match?: string;
  lookup_ip_match?: string;
  claude_response_ip?: string | null;
  country: string | null;
  country_code: string | null;
  asn: number | null;
  asn_org: string | null;
  company_type: string | null;
  is_residential: boolean | null;
  is_datacenter: boolean | null;
  is_vpn: boolean | null;
  is_proxy: boolean | null;
  is_tor: boolean | null;
  abuser_score: string | null;
  ai_verdict?: { label?: string; confidence?: number; reasoning?: string } | null;
  sources: string[] | null;
  checked_at: string | null;
  last_success_at: string | null;
  next_due: string | null;
  updated_at: string | null;
  /**
   * Combined Net.Coffee payload, when the backend exposes it. The Claude
   * source is described by the row fields above; `lookup` is the
   * comprehensive source and is rendered separately.
   */
  ip_info?: unknown;
}

export interface NodeScoreView {
  id: string;
  kind: 'static' | 'provider';
  nodeName: string;
  providerName: string;
  status: 'pending' | 'checking' | 'ok' | 'error' | 'skipped';
  score: number | null;
  scoreSource: string;
  ipMatch: IpMatch;
  ipMismatch: boolean;
  rangeOnly: boolean;
  exitIp: string | null;
  exitIpCidr: string | null;
  queriedIp: string | null;
  matchedIp: string | null;
  sources: SourceIntel[];
  checkedAt: string | null;
  lastSuccessAt: string | null;
  nextDue: string | null;
  updatedAt: string | null;
  error: string;
  errorCode: string;
}

export function toNodeScoreView(row: NodeScore): NodeScoreView {
  let sources = parseScoreSources(row.ip_info, row.range_only ? 'cidr' : undefined);
  const claudeSource = sources.find((s) => s.source === 'claude');
  const lookupSource = sources.find((s) => s.source === 'lookup');

  // The stored ip_info usually carries the Claude summary, but older rows or
  // partial successes may only expose row-level fields. Synthesize the Claude
  // card from them so the score never disappears or becomes zero.
  if (!claudeSource) {
    sources = [syntheticClaudeSource(row, lookupSource?.ipMatch), ...sources];
  } else {
    // The Claude summary in ip_info is trimmed; fill missing display fields
    // from the row without touching the lookup source's own data.
    sources = sources.map((source) =>
      source.source === 'claude'
        ? { ...source, intel: mergeRowIntel(source.intel, row) }
        : source
    );
  }

  const match = normalizeIpMatch(
    row.ip_mismatch
      ? 'mismatch'
      : row.range_only
        ? 'cidr'
        : row.claude_ip_match || (row.matched_ip ? 'exact' : 'unknown')
  );

  return {
    id: row.id,
    kind: row.node_kind === 'provider' ? 'provider' : 'static',
    nodeName: row.node_name,
    providerName: row.provider_name || '',
    status: normalizeScoreStatus(row.status),
    score:
      typeof row.trust_score === 'number' && Number.isFinite(row.trust_score)
        ? row.trust_score
        : null,
    scoreSource: row.score_source || '',
    ipMatch: match,
    ipMismatch: !!row.ip_mismatch,
    rangeOnly: !!row.range_only,
    exitIp: row.exit_ip || null,
    exitIpCidr: row.exit_ip_cidr || null,
    queriedIp: row.queried_ip || null,
    matchedIp: row.matched_ip || row.claude_response_ip || null,
    sources,
    checkedAt: normalizeTimestamp(row.checked_at),
    lastSuccessAt: normalizeTimestamp(row.last_success_at),
    nextDue: normalizeTimestamp(row.next_due),
    updatedAt: normalizeTimestamp(row.updated_at),
    error: row.error || '',
    errorCode: row.error_code || '',
  };
}

function mergeRowIntel(intel: IpIntel, row: NodeScore): IpIntel {
  return {
    ...intel,
    country: intel.country ?? row.country,
    countryCode: intel.countryCode ?? row.country_code,
    asn: intel.asn ?? row.asn,
    asnOrg: intel.asnOrg ?? row.asn_org,
    companyType: intel.companyType ?? row.company_type,
    isResidential: intel.isResidential ?? row.is_residential,
    isDatacenter: intel.isDatacenter ?? row.is_datacenter,
    isVpn: intel.isVpn ?? row.is_vpn,
    isProxy: intel.isProxy ?? row.is_proxy,
    isTor: intel.isTor ?? row.is_tor,
    abuserScore: intel.abuserScore ?? row.abuser_score,
    trustScore: intel.trustScore ?? row.trust_score,
  };
}

function syntheticClaudeSource(row: NodeScore, lookupMatch?: IpMatch): SourceIntel {
  const intel: IpIntel = normalizeIpInfo({
    country: row.country,
    countryCode: row.country_code,
    asn: row.asn,
    asOrganization: row.asn_org,
    company_type: row.company_type,
    is_residential: row.is_residential,
    is_datacenter: row.is_datacenter,
    is_vpn: row.is_vpn,
    is_proxy: row.is_proxy,
    is_tor: row.is_tor,
    abuser_score: row.abuser_score,
    trust_score: row.trust_score,
  });
  return {
    source: 'claude',
    queryIp: row.queried_ip || row.exit_ip || null,
    responseIp: row.matched_ip || row.claude_response_ip || null,
    ipMatch: lookupMatch === 'mismatch' ? 'unknown' : normalizeIpMatch(row.claude_ip_match ?? 'unknown'),
    cidr: row.exit_ip_cidr,
    trustScore:
      typeof row.trust_score === 'number' && Number.isFinite(row.trust_score)
        ? row.trust_score
        : null,
    intel,
  };
}

function normalizeScoreStatus(value: string): NodeScoreView['status'] {
  if (value === 'testing') return 'checking';
  if (value === 'failed') return 'error';
  if (value === 'checking' || value === 'ok' || value === 'error' || value === 'skipped') {
    return value;
  }
  return 'pending';
}

export interface BatchRefreshResult {
  queued: number;
  running: number;
  skipped: number;
}

export const nodeScoresApi = {
  list: () => api.get('node-scores').json<{ data: NodeScore[] }>(),
  get: (id: string) => api.get(`node-scores/${encodeURIComponent(id)}`).json<{ data: NodeScore }>(),
  refresh: (id: string) =>
    api
      .post(`node-scores/${encodeURIComponent(id)}/refresh`, { timeout: 120_000 })
      .json<{ data: BatchRefreshResult }>(),
  refreshBatch: (ids?: string[]) =>
    api
      .post('node-scores/refresh', {
        json: ids && ids.length ? { ids } : { all: true },
        timeout: 120_000,
      })
      .json<{ data: BatchRefreshResult }>(),
};
