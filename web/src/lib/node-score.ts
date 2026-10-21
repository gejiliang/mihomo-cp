/**
 * Helpers for rendering the third-party IP payloads without inventing values.
 *
 * The backend calls two public Net.Coffee endpoints independently:
 *   - `claude` = /api/iprisk/{ip} (Claude Trust Score)
 *   - `lookup` = /api/ip/lookup/{ip} (comprehensive IP intelligence)
 * Each response carries its own response IP and match scope. They are shown
 * separately; the UI never merges them into a single invented verdict.
 */

export type ScoreBand = 'high' | 'medium' | 'low' | 'veryLow';
export type IpMatch = 'exact' | 'cidr' | 'mismatch' | 'unknown';

/** Higher is better. Bands are descriptive, not a safety guarantee. */
export function scoreBand(score: number | null | undefined): ScoreBand | null {
  if (score === null || score === undefined || !Number.isFinite(score)) return null;
  if (score >= 80) return 'high';
  if (score >= 50) return 'medium';
  if (score >= 25) return 'low';
  return 'veryLow';
}

export function normalizeIpMatch(value: unknown): IpMatch {
  return value === 'exact' || value === 'cidr' || value === 'mismatch' ? value : 'unknown';
}

export interface IpIntel {
  ip: string | null;
  cidr: string | null;
  country: string | null;
  countryCode: string | null;
  region: string | null;
  city: string | null;
  asn: number | null;
  asnOrg: string | null;
  companyType: string | null;
  companyName: string | null;
  datacenterName: string | null;
  isResidential: boolean | null;
  isDatacenter: boolean | null;
  isVpn: boolean | null;
  isProxy: boolean | null;
  isTor: boolean | null;
  isMobile: boolean | null;
  isCrawler: boolean | null;
  isAbuser: boolean | null;
  abuserScore: string | null;
  trustScore: number | null;
  sources: string[];
  aiVerdictLabel: string | null;
  aiVerdictReason: string | null;
  raw: Record<string, unknown> | null;
}

export interface SourceIntel {
  source: 'claude' | 'lookup';
  queryIp: string | null;
  responseIp: string | null;
  ipMatch: IpMatch;
  cidr: string | null;
  trustScore: number | null;
  intel: IpIntel;
}

const EMPTY_INTEL: IpIntel = {
  ip: null,
  cidr: null,
  country: null,
  countryCode: null,
  region: null,
  city: null,
  asn: null,
  asnOrg: null,
  companyType: null,
  companyName: null,
  datacenterName: null,
  isResidential: null,
  isDatacenter: null,
  isVpn: null,
  isProxy: null,
  isTor: null,
  isMobile: null,
  isCrawler: null,
  isAbuser: null,
  abuserScore: null,
  trustScore: null,
  sources: [],
  aiVerdictLabel: null,
  aiVerdictReason: null,
  raw: null,
};

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

function str(v: unknown): string | null {
  return typeof v === 'string' && v.trim() ? v : null;
}

function num(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim()) {
    const parsed = Number(v);
    if (Number.isFinite(parsed)) return parsed;
  }
  return null;
}

function bool(v: unknown): boolean | null {
  return typeof v === 'boolean' ? v : null;
}

/** Normalizes one flat source object (no merging across sources). */
export function normalizeIpInfo(raw: unknown): IpIntel {
  const c = asRecord(raw);
  if (!c) return { ...EMPTY_INTEL };

  const sources = new Set<string>();
  const src = str(c.src);
  if (src) sources.add(src);
  const rawSources = c.sources ?? c.geo_sources;
  if (Array.isArray(rawSources)) {
    for (const s of rawSources) {
      if (typeof s === 'string') sources.add(s);
      else {
        const rec = asRecord(s);
        const name = rec ? str(rec.src) ?? str(rec.name) ?? str(rec.source) : null;
        if (name) sources.add(name);
      }
    }
  }

  const verdict = asRecord(c.ai_verdict);

  return {
    ip: str(c.ip),
    cidr: str(c.cidr),
    country: str(c.country),
    countryCode: str(c.countryCode) ?? str(c.country_code),
    region: str(c.region),
    city: str(c.city),
    asn: num(c.asn),
    asnOrg:
      str(c.asOrganization) ??
      str(c.as_organization) ??
      str(c.asn_org) ??
      str(c.asnOrg),
    companyType: str(c.company_type) ?? str(c.companyType),
    companyName: str(c.company_name) ?? str(c.companyName) ?? str(c.isp),
    datacenterName: str(c.datacenter_name) ?? str(c.datacenterName),
    isResidential: bool(c.isResidential) ?? bool(c.is_residential),
    isDatacenter: bool(c.is_datacenter) ?? bool(c.isDatacenter),
    isVpn: bool(c.is_vpn) ?? bool(c.isVpn),
    isProxy: bool(c.is_proxy) ?? bool(c.isProxy),
    isTor: bool(c.is_tor) ?? bool(c.isTor),
    isMobile: bool(c.is_mobile) ?? bool(c.isMobile),
    isCrawler: bool(c.is_crawler) ?? bool(c.isCrawler),
    isAbuser: bool(c.is_abuser) ?? bool(c.isAbuser),
    abuserScore: str(c.abuser_score) ?? str(c.abuserScore),
    trustScore: num(c.trust_score) ?? num(c.trustScore),
    sources: [...sources],
    aiVerdictLabel: verdict ? str(verdict.label) : null,
    aiVerdictReason: verdict ? str(verdict.reasoning) : null,
    raw: c,
  };
}

/**
 * Splits a combined `ip_info` payload into its independent sources.
 *
 * Accepted shapes:
 *  - `{ claude: {...}, lookup: {...} }` (preferred)
 *  - `{ iprisk: {...}, lookup: {...} }`
 *  - a single top-level response from either endpoint
 *
 * The source is only inferred from unambiguous markers (`ai_verdict`,
 * `is_bogon`, `intelligence` vs `timezone`, `rpki_status`). Otherwise the
 * caller's row-level `claude_score` is displayed alone.
 */
export function parseScoreSources(raw: unknown, parentMatch?: unknown): SourceIntel[] {
  const root = asRecord(raw);
  if (!root) return [];
  const out: SourceIntel[] = [];

  const claudeRaw =
    asRecord(root.claude) ??
    asRecord(root.claude_risk) ??
    asRecord(root.iprisk) ??
    null;
  const lookupRaw =
    asRecord(root.lookup) ??
    asRecord(root.ip_lookup) ??
    asRecord(root.ipLookup) ??
    null;

  if (claudeRaw) out.push(buildSource('claude', claudeRaw, parentMatch));
  if (lookupRaw) out.push(buildSource('lookup', lookupRaw, parentMatch));

  if (out.length === 0) {
    // Combined payload with the two sources flattened? Avoid claiming a
    // source based only on a shared trust_score field.
    const looksLikeClaude =
      root.ai_verdict === undefined &&
      (root.timezone !== undefined || root.rpki_status !== undefined || root.as_organization !== undefined);
    const looksLikeLookup =
      root.ai_verdict !== undefined || root.is_bogon !== undefined || root.intelligence !== undefined;
    if (looksLikeClaude) out.push(buildSource('claude', root, parentMatch));
    else if (looksLikeLookup) out.push(buildSource('lookup', root, parentMatch));
  }

  return out;
}

function buildSource(
  source: 'claude' | 'lookup',
  obj: Record<string, unknown>,
  parentMatch?: unknown
): SourceIntel {
  const intel = normalizeIpInfo(obj);
  const explicit = obj.ip_match ?? parentMatch;
  let match = normalizeIpMatch(explicit);
  if (match === 'unknown') {
    const query = str(obj.query_ip);
    const response = intel.ip;
    if (query && response) {
      match = query === response ? 'exact' : 'mismatch';
    }
  }
  return {
    source,
    queryIp: str(obj.query_ip),
    responseIp: intel.ip,
    ipMatch: match,
    cidr: intel.cidr,
    trustScore: intel.trustScore,
    intel,
  };
}
