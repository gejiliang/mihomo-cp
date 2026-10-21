import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SubscriptionCard } from './subscription-card';
import type { Subscription } from '@/api/subscriptions';
import { useI18nStore } from '@/i18n';

const BASE: Subscription = {
  id: 'sub-1',
  name: 'Example subscription',
  enabled: true,
  status: 'ok',
  last_error: '',
  provider_name: 'provider-example',
  provider_source: 'created',
  provider_exists: true,
  url_masked: 'https://sub.example.com/••••',
  node_count: null,
  source_type: 'native',
  traffic_provider_name: '',
  supplier_reset_day: null,
  quota_bytes: null,
  used_bytes: null,
  remaining_bytes: null,
  usage_source: 'none',
  local_used_bytes: null,
  expires_at: null,
  expires_source: 'unknown',
  reset_day: 10,
  reset_hour: 12,
  reset_minute: 0,
  timezone: 'Asia/Shanghai',
  cycle_start: null,
  cycle_end: null,
  next_reset: null,
  userinfo_fetched_at: null,
  last_refresh_at: null,
  last_success_at: null,
  stale: false,
  userinfo: null,
  traffic_provider_id: '',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
};

function renderCard(subscription: Subscription, isAdmin = true) {
  return render(
    <MemoryRouter>
      <SubscriptionCard
        subscription={subscription}
        refreshing={false}
        isAdmin={isAdmin}
        onRefresh={vi.fn()}
        onEdit={vi.fn()}
      />
    </MemoryRouter>
  );
}

describe('SubscriptionCard', () => {
  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
  });

  it('shows unknown values instead of zero or unlimited', () => {
    renderCard(BASE);
    expect(screen.getAllByText('Unknown').length).toBeGreaterThan(0);
    expect(screen.getByText('Not reported')).toBeInTheDocument();
    expect(screen.queryByText('0 B')).not.toBeInTheDocument();
    expect(screen.queryByText(/unlimited/i)).not.toBeInTheDocument();
  });

  it('renders vendor quota, remaining and the exact next reset with timezone', () => {
    renderCard({
      ...BASE,
      quota_bytes: 200 * 1024 ** 3,
      used_bytes: 50 * 1024 ** 3,
      remaining_bytes: 150 * 1024 ** 3,
      usage_source: 'vendor',
      userinfo_fetched_at: '2026-10-21T03:00:00Z',
      last_refresh_at: '2026-10-21T03:00:00Z',
      last_success_at: '2026-10-21T03:00:00Z',
      expires_at: '2027-01-09T04:00:00Z',
      expires_source: 'vendor',
      next_reset: '2026-11-10T04:00:00Z',
      node_count: 42,
      userinfo: {
        upload: 10 * 1024 ** 3,
        download: 40 * 1024 ** 3,
        total: 200 * 1024 ** 3,
        expire: 1800000000,
        fetched_at: '2026-10-21T03:00:00Z',
      },
    });
    expect(screen.getByText('150 GiB')).toBeInTheDocument();
    expect(screen.getByText(/200 GiB/)).toBeInTheDocument();
    expect(screen.getByText(/10\/11\/2026, 12:00 \(Asia\/Shanghai\)/)).toBeInTheDocument();
    expect(screen.getByText(/09\/01\/2027, 12:00 \(Asia\/Shanghai\)/)).toBeInTheDocument();
    expect(screen.getByText(/Nodes:\s*42/)).toBeInTheDocument();
    expect(screen.getByText('Vendor reported')).toBeInTheDocument();
  });

  it('never renders the raw subscription URL', () => {
    renderCard(BASE);
    expect(screen.getByText('https://sub.example.com/••••')).toBeInTheDocument();
    expect(screen.queryByText(/token/i)).not.toBeInTheDocument();
  });

  it('shows stale and last error messages', () => {
    renderCard({
      ...BASE,
      status: 'stale',
      stale: true,
      last_error: 'upstream HTTP 403',
      last_success_at: '2026-10-20T00:00:00Z',
    });
    expect(screen.getByText('Stale data')).toBeInTheDocument();
    expect(screen.getByText(/upstream HTTP 403/)).toBeInTheDocument();
    expect(screen.getByText(/older than 24 hours/)).toBeInTheDocument();
  });

  it('explains that pausing auto-update does not stop published nodes', () => {
    renderCard({ ...BASE, enabled: false, status: 'disabled' });
    expect(screen.getByText('Auto-update paused')).toBeInTheDocument();
    expect(
      screen.getByText(/Already-published nodes can still carry traffic/i)
    ).toBeInTheDocument();
    expect(screen.queryByText(/^Disabled$/)).not.toBeInTheDocument();
  });

  it('disables mutations for readonly users', () => {
    renderCard(BASE, false);
    const buttons = screen.getAllByRole('button');
    for (const button of buttons) {
      expect(button).toBeDisabled();
    }
  });
});

describe('SubscriptionCard · traffic source', () => {
  const TRAFFIC: Subscription = {
    ...BASE,
    source_type: 'traffic',
    provider_name: '',
    provider_source: '',
    provider_exists: false,
    traffic_provider_name: 'Legacy Traffic',
    node_count: 6,
    status: 'local',
    url_masked: '',
    usage_source: 'vendor',
    quota_bytes: 1000 * 1024 ** 3,
    used_bytes: 115 * 1024 ** 3,
    remaining_bytes: 885 * 1024 ** 3,
    supplier_reset_day: 20,
    reset_day: 20,
    reset_hour: 0,
    reset_minute: 0,
    timezone: null,
  };

  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
  });

  it('labels the source as static and names the traffic provider, without a native-provider-missing warning', () => {
    renderCard(TRAFFIC);
    expect(screen.getByText('Static source')).toBeInTheDocument();
    expect(screen.getByText(/Traffic source: Legacy Traffic/)).toBeInTheDocument();
    expect(screen.getByText(/Matched static nodes/)).toBeInTheDocument();
    expect(screen.queryByText(/missing from the config draft/i)).not.toBeInTheDocument();
  });

  it('renders the vendor aggregate usage and local-only status without invented directions', () => {
    renderCard(TRAFFIC);
    expect(screen.getByText('Vendor reported')).toBeInTheDocument();
    expect(screen.getByText('Local data')).toBeInTheDocument();
    expect(screen.getByText(/no subscription link/i)).toBeInTheDocument();
  });

  it('shows the supplier reset day even when a local reset day is configured, without implying vendor time/zone', () => {
    renderCard(TRAFFIC);
    expect(screen.getByText(/Vendor reports reset day 20 \(day only — no hour or timezone\)/i)).toBeInTheDocument();
    expect(screen.getByText(/locally configured, not supplied by the vendor/i)).toBeInTheDocument();
  });

  it('uses traffic-specific paused and delete wording', () => {
    renderCard({ ...TRAFFIC, enabled: false, status: 'disabled' });
    expect(screen.getByText(/Automatic metadata refresh is paused\. The traffic source and static config are untouched\./i)).toBeInTheDocument();
  });
});
