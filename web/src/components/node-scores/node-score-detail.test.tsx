import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NodeScoreDetail } from './node-score-detail';
import type { NodeScore } from '@/api/node-scores';
import { useI18nStore } from '@/i18n';

const BASE: NodeScore = {
  id: 'provider/provider-example/example-node',
  node_name: 'example-node',
  node_kind: 'provider',
  provider_name: 'provider-example',
  status: 'ok',
  error: '',
  error_code: '',
  exit_ip: '203.0.113.10',
  exit_ip_cidr: '203.0.113.0/24',
  exit_ip_source: 'claude-trace',
  trust_score: 91,
  score_source: 'netcoffee-claude',
  score_url: 'https://ip.net.coffee/claude/',
  detail_url: 'https://ip.net.coffee/ip/203.0.113.10',
  queried_ip: '203.0.113.10',
  matched_ip: '203.0.113.10',
  ip_mismatch: false,
  range_only: false,
  country: 'United States',
  country_code: 'US',
  asn: 64500,
  asn_org: 'Example Hosting LLC',
  company_type: 'hosting',
  is_residential: false,
  is_datacenter: true,
  is_vpn: false,
  is_proxy: false,
  is_tor: false,
  abuser_score: '0.0019 (Low)',
  sources: ['netcoffee-claude', 'netcoffee-ip'],
  checked_at: '2026-10-21T03:00:00Z',
  last_success_at: '2026-10-21T03:00:00Z',
  next_due: '2026-10-28T03:00:00Z',
  updated_at: '2026-10-21T03:00:00Z',
  ip_info: {
    claude: {
      query_ip: '203.0.113.10',
      ip: '203.0.113.10',
      ip_match: 'exact',
      cidr: '203.0.113.0/24',
      trust_score: 91,
      country: 'United States',
      countryCode: 'US',
      asn: 64500,
      asOrganization: 'Example Hosting LLC',
      is_datacenter: true,
      isResidential: false,
    },
    lookup: {
      query_ip: '203.0.113.10',
      ip: '203.0.113.11',
      ip_match: 'cidr',
      cidr: '203.0.113.0/24',
      trust_score: 45,
      country: 'United States',
      countryCode: 'US',
      asn: 64501,
      asOrganization: 'Example ISP',
      is_datacenter: false,
      isResidential: true,
      ai_verdict: { label: 'Residential range', reasoning: 'Range-level sample' },
    },
  },
};

function renderDetail(overrides: Partial<NodeScore> = {}, isAdmin = true) {
  const onRefresh = vi.fn();
  render(
    <NodeScoreDetail
      score={{ ...BASE, ...overrides }}
      open
      onOpenChange={vi.fn()}
      isAdmin={isAdmin}
      refreshing={false}
      onRefresh={onRefresh}
    />
  );
  return { onRefresh };
}

describe('NodeScoreDetail', () => {
  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
  });

  it('shows the Claude and comprehensive sources separately', () => {
    renderDetail();
    expect(screen.getAllByText('Claude source · /api/iprisk').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Comprehensive source · /api/ip/lookup').length).toBeGreaterThan(0);
    // Claude score from the row and lookup score from its own payload.
    expect(screen.getByText('91')).toBeInTheDocument();
    expect(screen.getByText('45')).toBeInTheDocument();
    expect(screen.getByText('Residential range')).toBeInTheDocument();
  });

  it('never presents a missing score as zero', () => {
    renderDetail({ trust_score: null, ip_info: undefined });
    expect(screen.getAllByText('No score').length).toBeGreaterThan(0);
  });

  it('surfaces range-only results instead of claiming an exact match', () => {
    renderDetail({
      trust_score: 64,
      queried_ip: '203.0.113.10',
      matched_ip: '203.0.113.11',
      range_only: true,
      ip_info: {
        claude: {
          query_ip: '203.0.113.10',
          ip: '203.0.113.11',
          ip_match: 'cidr',
          trust_score: 64,
        },
      },
    });
    expect(screen.getAllByText('Declared range (not exact)').length).toBeGreaterThan(0);
    expect(screen.getByText(/neighbor IP inside the declared range/i)).toBeInTheDocument();
  });

  it('surfaces mismatched IPs with an explicit warning', () => {
    renderDetail({
      trust_score: 100,
      queried_ip: '203.0.113.10',
      matched_ip: '198.51.100.7',
      ip_mismatch: true,
      ip_info: {
        claude: {
          query_ip: '203.0.113.10',
          ip: '198.51.100.7',
          ip_match: 'mismatch',
          trust_score: 100,
        },
      },
    });
    expect(screen.getByText(/returned data for a different address/i)).toBeInTheDocument();
  });

  it('states that browser DNS/STUN were not tested', () => {
    renderDetail();
    expect(screen.getByText(/Browser DNS, STUN and WebRTC are not tested/i)).toBeInTheDocument();
  });

  it('disables refresh for readonly users', () => {
    renderDetail({}, false);
    expect(screen.getByRole('button', { name: /Refresh score/ })).toBeDisabled();
  });
});
