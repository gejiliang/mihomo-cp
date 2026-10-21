import { describe, expect, it } from 'vitest';
import { normalizeIpMatch, normalizeIpInfo, parseScoreSources, scoreBand } from './node-score';

describe('scoreBand', () => {
  it('does not invent a band for missing scores', () => {
    expect(scoreBand(null)).toBeNull();
    expect(scoreBand(undefined)).toBeNull();
  });

  it('maps high scores to high trust', () => {
    expect(scoreBand(100)).toBe('high');
    expect(scoreBand(80)).toBe('high');
    expect(scoreBand(79)).toBe('medium');
    expect(scoreBand(50)).toBe('medium');
    expect(scoreBand(49)).toBe('low');
    expect(scoreBand(24)).toBe('veryLow');
    expect(scoreBand(0)).toBe('veryLow');
  });
});

describe('normalizeIpInfo', () => {
  it('returns nulls for an empty payload', () => {
    const info = normalizeIpInfo(null);
    expect(info.asn).toBeNull();
    expect(info.isResidential).toBeNull();
    expect(info.trustScore).toBeNull();
    expect(info.sources).toEqual([]);
  });

  it('reads the flat public response shape', () => {
    const info = normalizeIpInfo({
      ip: '203.0.113.10',
      cidr: '203.0.113.0/24',
      asn: 64500,
      asOrganization: 'Example Hosting',
      country: 'United States',
      countryCode: 'US',
      is_datacenter: true,
      isResidential: false,
      is_vpn: false,
      is_proxy: true,
      is_tor: false,
      trust_score: 100,
      abuser_score: '0.0019 (Low)',
      src: 'g1',
      ai_verdict: { label: 'Clean residential', confidence: 95, reasoning: 'No DC markers' },
    });
    expect(info.ip).toBe('203.0.113.10');
    expect(info.asn).toBe(64500);
    expect(info.isDatacenter).toBe(true);
    expect(info.isResidential).toBe(false);
    expect(info.isProxy).toBe(true);
    expect(info.isTor).toBe(false);
    expect(info.trustScore).toBe(100);
    expect(info.aiVerdictLabel).toBe('Clean residential');
    expect(info.sources).toEqual(['g1']);
  });

  it('preserves false booleans instead of treating them as unknown', () => {
    const info = normalizeIpInfo({ is_vpn: false, is_tor: false, is_datacenter: false });
    expect(info.isVpn).toBe(false);
    expect(info.isTor).toBe(false);
    expect(info.isDatacenter).toBe(false);
    expect(info.isResidential).toBeNull();
  });

  it('reads Go-side snake_case fields from the Claude source', () => {
    const info = normalizeIpInfo({
      ip: '203.0.113.11',
      as_organization: 'Example ISP',
      country_code: 'us',
      is_residential: true,
      is_datacenter: false,
    });
    expect(info.asnOrg).toBe('Example ISP');
    expect(info.countryCode).toBe('us');
    expect(info.isResidential).toBe(true);
    expect(info.isDatacenter).toBe(false);
  });
});

describe('parseScoreSources', () => {
  it('keeps Claude and lookup results separate', () => {
    const sources = parseScoreSources({
      claude: {
        query_ip: '203.0.113.11',
        ip: '203.0.113.11',
        ip_match: 'exact',
        trust_score: 92,
        cidr: '203.0.113.0/24',
      },
      lookup: {
        query_ip: '203.0.113.11',
        ip: '203.0.113.12',
        ip_match: 'cidr',
        trust_score: 40,
        asOrganization: 'Example ISP',
      },
    });
    expect(sources.map((s) => s.source)).toEqual(['claude', 'lookup']);
    expect(sources[0].trustScore).toBe(92);
    expect(sources[0].ipMatch).toBe('exact');
    expect(sources[1].trustScore).toBe(40);
    expect(sources[1].ipMatch).toBe('cidr');
  });

  it('never merges a mismatch into an exact score', () => {
    const sources = parseScoreSources({
      claude: { query_ip: '203.0.113.11', ip: '198.51.100.7', ip_match: 'mismatch', trust_score: 100 },
    });
    expect(sources).toHaveLength(1);
    expect(sources[0].ipMatch).toBe('mismatch');
    expect(sources[0].intel.ip).toBe('198.51.100.7');
  });

  it('accepts a single flat source without inventing a second one', () => {
    const claude = parseScoreSources({
      query_ip: '203.0.113.11',
      ip: '203.0.113.11',
      trust_score: 77,
      timezone: 'UTC',
    });
    expect(claude).toHaveLength(1);
    expect(claude[0].source).toBe('claude');

    const lookup = parseScoreSources({
      ip: '203.0.113.11',
      ai_verdict: { label: 'Clean' },
      is_bogon: false,
    });
    expect(lookup).toHaveLength(1);
    expect(lookup[0].source).toBe('lookup');
  });

  it('does not guess when the payload is unknown', () => {
    expect(parseScoreSources(null)).toEqual([]);
    expect(parseScoreSources({})).toEqual([]);
  });
});

describe('normalizeIpMatch', () => {
  it('defaults unknown values to unknown', () => {
    expect(normalizeIpMatch('exact')).toBe('exact');
    expect(normalizeIpMatch('cidr')).toBe('cidr');
    expect(normalizeIpMatch('mismatch')).toBe('mismatch');
    expect(normalizeIpMatch('')).toBe('unknown');
    expect(normalizeIpMatch(null)).toBe('unknown');
  });
});

// ── Node score row mapper ───────────────────────────────────────────────────
import { toNodeScoreView, type NodeScore } from '@/api/node-scores';

const BASE_ROW: NodeScore = {
  id: 'provider/provider-example/example-node',
  node_name: 'example-node',
  node_kind: 'provider',
  provider_name: 'provider-example',
  status: 'pending',
  error: '',
  error_code: '',
  exit_ip: '',
  exit_ip_cidr: null,
  exit_ip_source: '',
  trust_score: null,
  score_source: 'netcoffee-claude',
  score_url: 'https://ip.net.coffee/claude/',
  detail_url: 'https://ip.net.coffee/ip/203.0.113.10',
  queried_ip: null,
  matched_ip: null,
  ip_mismatch: false,
  range_only: false,
  country: null,
  country_code: null,
  asn: null,
  asn_org: null,
  company_type: null,
  is_residential: null,
  is_datacenter: null,
  is_vpn: null,
  is_proxy: null,
  is_tor: null,
  abuser_score: null,
  sources: null,
  checked_at: '0001-01-01T00:00:00Z',
  last_success_at: null,
  next_due: null,
  updated_at: null,
};

describe('toNodeScoreView', () => {
  it('keeps a missing score and zero timestamps unknown', () => {
    const view = toNodeScoreView(BASE_ROW);
    expect(view.score).toBeNull();
    expect(view.exitIp).toBeNull();
    expect(view.checkedAt).toBeNull();
    expect(view.status).toBe('pending');
  });

  it('does not turn an absent score into zero', () => {
    const view = toNodeScoreView({
      ...BASE_ROW,
      status: 'ok',
      exit_ip: '203.0.113.10',
      matched_ip: '203.0.113.10',
      queried_ip: '203.0.113.10',
    });
    expect(view.score).toBeNull();
    expect(view.ipMatch).toBe('exact');
  });

  it('marks a same-range answer as range-only, not exact', () => {
    const view = toNodeScoreView({
      ...BASE_ROW,
      status: 'ok',
      trust_score: 64,
      queried_ip: '203.0.113.10',
      matched_ip: '203.0.113.11',
      range_only: true,
    });
    expect(view.ipMatch).toBe('cidr');
    expect(view.rangeOnly).toBe(true);
    expect(view.matchedIp).toBe('203.0.113.11');
  });

  it('marks an unrelated answer as mismatch', () => {
    const view = toNodeScoreView({
      ...BASE_ROW,
      status: 'ok',
      trust_score: 100,
      queried_ip: '203.0.113.10',
      matched_ip: '198.51.100.7',
      ip_mismatch: true,
    });
    expect(view.ipMatch).toBe('mismatch');
    expect(view.ipMismatch).toBe(true);
  });

  it('exposes both Net.Coffee sources separately', () => {
    const view = toNodeScoreView({
      ...BASE_ROW,
      status: 'ok',
      trust_score: 91,
      matched_ip: '203.0.113.10',
      queried_ip: '203.0.113.10',
      ip_info: {
        claude: {
          query_ip: '203.0.113.10',
          ip: '203.0.113.10',
          ip_match: 'exact',
          trust_score: 91,
          cidr: '203.0.113.0/24',
        },
        lookup: {
          query_ip: '203.0.113.10',
          ip: '203.0.113.11',
          ip_match: 'cidr',
          trust_score: 45,
          ai_verdict: { label: 'Hosting range' },
        },
      },
    });
    expect(view.score).toBe(91);
    expect(view.sources).toHaveLength(2);
    expect(view.sources[0].source).toBe('claude');
    expect(view.sources[0].ipMatch).toBe('exact');
    expect(view.sources[1].source).toBe('lookup');
    expect(view.sources[1].ipMatch).toBe('cidr');
    expect(view.sources[1].intel.aiVerdictLabel).toBe('Hosting range');
  });
});
