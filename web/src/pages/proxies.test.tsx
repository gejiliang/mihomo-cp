import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProxiesPage from './proxies';
import type { Proxy } from '@/api/proxies';
import type { RuntimeProxyProvider } from '@/api/runtime';
import { useI18nStore } from '@/i18n';

const { list, proxyProviders } = vi.hoisted(() => ({ list: vi.fn(), proxyProviders: vi.fn() }));

vi.mock('@/api/proxies', () => ({ proxiesApi: { list } }));
vi.mock('@/api/runtime', () => ({ runtimeApi: { proxyProviders } }));

const STATIC_PROXY: Proxy = {
  id: '11111111-1111-4111-8111-111111111111',
  name: 'Static HK 01',
  type: 'ss',
  country: 'HK',
  config: { server: '198.51.100.10', port: 8388, udp: true },
  sort_order: 0,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
};

const LOADED_PROVIDER: RuntimeProxyProvider = {
  name: 'Demo Provider',
  type: 'Proxy',
  vehicle_type: 'HTTP',
  status: 'ok',
  updated_at: '2026-10-20T08:00:00Z',
  node_count: 1,
  nodes: [
    {
      id: 'provider/Demo%20Provider/Demo%20HK%20%2F%2001',
      name: 'Demo HK / 01',
      type: 'AnyTLS',
      provider: 'Demo Provider',
      alive: true,
      udp: true,
      delay: null,
    },
  ],
};

const BROKEN_PROVIDER: RuntimeProxyProvider = {
  name: 'Broken Provider',
  type: 'Proxy',
  vehicle_type: 'HTTP',
  status: 'unknown',
  updated_at: '',
  node_count: null,
  nodes: null,
};

function renderPage() {
  return render(
    <MemoryRouter>
      <ProxiesPage />
    </MemoryRouter>
  );
}

describe('ProxiesPage node inventory', () => {
  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
    list.mockReset();
    proxyProviders.mockReset();
    list.mockResolvedValue({ data: [STATIC_PROXY] });
  });

  it('keeps the static list usable when the subscription inventory fails', async () => {
    proxyProviders.mockRejectedValue(new Error('mihomo unreachable'));
    renderPage();

    expect(await screen.findByText('Static HK 01')).toBeInTheDocument();
    expect(await screen.findByText('Failed to load subscription nodes')).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
    // Static CRUD actions are unaffected by the provider failure.
    expect(screen.getByTitle('Edit')).toBeInTheDocument();
    expect(screen.getByTitle('Delete')).toBeInTheDocument();
  });

  it('shows loaded subscription nodes and unknown provider counts instead of zero', async () => {
    proxyProviders.mockResolvedValue({ data: { providers: [LOADED_PROVIDER, BROKEN_PROVIDER] } });
    renderPage();

    expect(await screen.findByText('Demo HK / 01')).toBeInTheDocument();
    expect(screen.getByText('Static HK 01')).toBeInTheDocument();
    expect(screen.getByText('Demo Provider: 1')).toBeInTheDocument();
    expect(screen.getByText('Broken Provider: Unknown')).toBeInTheDocument();
    expect(screen.queryByText('Broken Provider: 0')).not.toBeInTheDocument();
  });

  it('retries the subscription inventory without touching the static list', async () => {
    proxyProviders
      .mockRejectedValueOnce(new Error('mihomo unreachable'))
      .mockResolvedValueOnce({ data: { providers: [LOADED_PROVIDER] } });
    renderPage();

    expect(await screen.findByText('Failed to load subscription nodes')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Retry'));

    expect(await screen.findByText('Demo HK / 01')).toBeInTheDocument();
    expect(screen.queryByText('Failed to load subscription nodes')).not.toBeInTheDocument();
    expect(list).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Static HK 01')).toBeInTheDocument();
  });
});
