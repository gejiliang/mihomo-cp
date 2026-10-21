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
    expect(screen.getByText(/Vendor \(subscription-userinfo\)/)).toBeInTheDocument();
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

  it('disables mutations for readonly users', () => {
    renderCard(BASE, false);
    const buttons = screen.getAllByRole('button');
    for (const button of buttons) {
      expect(button).toBeDisabled();
    }
  });
});
