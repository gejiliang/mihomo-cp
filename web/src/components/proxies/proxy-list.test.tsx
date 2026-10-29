import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ProxyList } from './proxy-list';
import type { Proxy } from '@/api/proxies';
import type { RuntimeProviderNode, RuntimeProxyProvider } from '@/api/runtime';
import { buildNodeRows } from '@/lib/node-inventory';
import { useI18nStore } from '@/i18n';

const { proxyDelay, providerNodeHealthcheck } = vi.hoisted(() => ({
  proxyDelay: vi.fn(),
  providerNodeHealthcheck: vi.fn(),
}));

vi.mock('@/api/runtime', () => ({
  runtimeApi: { proxyDelay, providerNodeHealthcheck },
}));

function staticProxy(overrides: Partial<Proxy> = {}): Proxy {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    name: 'Static HK 01',
    type: 'ss',
    country: 'HK',
    config: { server: '198.51.100.10', port: 8388, udp: true },
    sort_order: 0,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...overrides,
  };
}

function providerNode(overrides: Partial<RuntimeProviderNode> = {}): RuntimeProviderNode {
  return {
    id: 'provider/Demo%20Provider/Demo%20HK%20%2F%2001%20%2B%20%231',
    name: 'Demo HK / 01 + #1',
    type: 'AnyTLS',
    provider: 'Demo Provider',
    alive: true,
    udp: true,
    delay: null,
    ...overrides,
  };
}

function provider(
  nodes: RuntimeProviderNode[] | null,
  overrides: Partial<RuntimeProxyProvider> = {}
): RuntimeProxyProvider {
  return {
    name: 'Demo Provider',
    type: 'Proxy',
    vehicle_type: 'HTTP',
    status: nodes === null ? 'unknown' : 'ok',
    updated_at: '2026-10-20T08:00:00Z',
    node_count: nodes === null ? null : nodes.length,
    nodes,
    ...overrides,
  };
}

function renderList(options: {
  providers?: RuntimeProxyProvider[];
  proxies?: Proxy[];
  emptyMessage?: string;
} = {}) {
  const proxies = options.proxies ?? [staticProxy()];
  const providers = options.providers ?? [provider([providerNode()])];
  const onEdit = vi.fn();
  const onDeleted = vi.fn();
  const onCopied = vi.fn();
  render(
    <ProxyList
      rows={buildNodeRows(proxies, providers)}
      providers={providers}
      onEdit={onEdit}
      onDeleted={onDeleted}
      onCopied={onCopied}
      emptyMessage={options.emptyMessage}
    />
  );
  return { onEdit, onDeleted, onCopied };
}

function rowFor(name: string): HTMLElement {
  const row = screen.getByText(name).closest('tr');
  if (!row) throw new Error(`no row for ${name}`);
  return row as HTMLElement;
}

describe('ProxyList (unified static + subscription nodes)', () => {
  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
    proxyDelay.mockReset();
    providerNodeHealthcheck.mockReset();
  });

  it('shows source labels and protocols for both sources', () => {
    renderList();

    expect(screen.getByText('Static HK 01')).toBeInTheDocument();
    expect(screen.getByText('Demo HK / 01 + #1')).toBeInTheDocument();
    expect(within(rowFor('Static HK 01')).getByText('Static')).toBeInTheDocument();
    expect(within(rowFor('Demo HK / 01 + #1')).getByText('Demo Provider')).toBeInTheDocument();
    // AnyTLS subscription nodes are shown with a readable protocol badge.
    expect(within(rowFor('Demo HK / 01 + #1')).getByText('AnyTLS')).toBeInTheDocument();
    // Provider rows never show a server address (country/server/delay are '—').
    expect(within(rowFor('Demo HK / 01 + #1')).getAllByText('—').length).toBeGreaterThanOrEqual(2);
  });

  it('keeps subscription nodes read-only while static CRUD actions still work', () => {
    const { onEdit } = renderList();

    const subscriptionRow = rowFor('Demo HK / 01 + #1');
    expect(within(subscriptionRow).queryByTitle('Edit')).not.toBeInTheDocument();
    expect(within(subscriptionRow).queryByTitle('Copy')).not.toBeInTheDocument();
    expect(within(subscriptionRow).queryByTitle('Delete')).not.toBeInTheDocument();
    expect(within(subscriptionRow).getByTitle('Details')).toBeInTheDocument();
    expect(within(subscriptionRow).getByTitle('Test Delay')).toBeInTheDocument();

    // Clicking a subscription row never opens the static edit form.
    fireEvent.click(within(subscriptionRow).getByText('Demo HK / 01 + #1'));
    expect(onEdit).not.toHaveBeenCalled();

    const staticRow = rowFor('Static HK 01');
    expect(within(staticRow).getByTitle('Edit')).toBeInTheDocument();
    expect(within(staticRow).getByTitle('Copy')).toBeInTheDocument();
    expect(within(staticRow).getByTitle('Delete')).toBeInTheDocument();
    fireEvent.click(within(staticRow).getByText('Static HK 01'));
    expect(onEdit).toHaveBeenCalledTimes(1);
    expect(onEdit.mock.calls[0][0]).toMatchObject({ id: '11111111-1111-4111-8111-111111111111' });
  });

  it('tests a subscription node through the provider-scoped healthcheck with its real name', async () => {
    providerNodeHealthcheck.mockResolvedValue({ data: { delay: 88 } });
    renderList({
      providers: [provider([providerNode({ name: 'Demo HK / 01 + #1 节点' })])],
    });

    const row = rowFor('Demo HK / 01 + #1 节点');
    fireEvent.click(within(row).getByTitle('Test Delay'));

    expect(providerNodeHealthcheck).toHaveBeenCalledTimes(1);
    // Provider + actual node name — not the row id/key, and never the global
    // /proxies/{name}/delay endpoint.
    expect(providerNodeHealthcheck).toHaveBeenCalledWith('Demo Provider', 'Demo HK / 01 + #1 节点');
    expect(proxyDelay).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText('88ms')).toBeInTheDocument());
  });

  it('tests a static node through the global runtime delay endpoint', async () => {
    proxyDelay.mockResolvedValue({ data: { delay: 33 } });
    renderList();

    fireEvent.click(within(rowFor('Static HK 01')).getByTitle('Test Delay'));

    expect(proxyDelay).toHaveBeenCalledWith('Static HK 01');
    expect(providerNodeHealthcheck).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText('33ms')).toBeInTheDocument());
  });

  it('keeps same-named nodes under different providers distinct when testing', async () => {
    providerNodeHealthcheck.mockResolvedValue({ data: { delay: 50 } });
    const providers = [
      provider([providerNode({ id: 'provider/Demo%20Provider/Shared%20Node', name: 'Shared Node' })]),
      provider(
        [
          providerNode({
            id: 'provider/Other%20Provider/Shared%20Node',
            name: 'Shared Node',
            provider: 'Other Provider',
          }),
        ],
        { name: 'Other Provider' }
      ),
    ];
    renderList({ proxies: [], providers });

    const rows = screen.getAllByText('Shared Node').map((cell) => cell.closest('tr') as HTMLElement);
    expect(rows).toHaveLength(2);
    fireEvent.click(within(rows[0]).getByTitle('Test Delay'));
    fireEvent.click(within(rows[1]).getByTitle('Test Delay'));

    expect(providerNodeHealthcheck.mock.calls).toEqual([
      ['Demo Provider', 'Shared Node'],
      ['Other Provider', 'Shared Node'],
    ]);
  });

  it('shows only safe metadata in the subscription node details dialog', () => {
    const unsafeNode = {
      ...providerNode({ name: 'Demo HK / 01', delay: 120 }),
      server: '203.0.113.10',
      password: 'super-secret',
      url: 'https://subscription.example.com/private-token',
    } as unknown as RuntimeProviderNode;
    renderList({ providers: [provider([unsafeNode])] });

    fireEvent.click(within(rowFor('Demo HK / 01')).getByTitle('Details'));

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('Node details')).toBeInTheDocument();
    expect(within(dialog).getByText('AnyTLS')).toBeInTheDocument();
    expect(within(dialog).getByText('120ms')).toBeInTheDocument();
    expect(within(dialog).getByText(/read-only/i)).toBeInTheDocument();

    const text = document.body.textContent ?? '';
    expect(text).not.toContain('203.0.113.10');
    expect(text).not.toContain('super-secret');
    expect(text).not.toContain('subscription.example.com');
    expect(text).not.toContain('private-token');
  });

  it('batch delay tests every visible node with its own delay method', async () => {
    proxyDelay.mockResolvedValue({ data: { delay: 60 } });
    providerNodeHealthcheck.mockResolvedValue({ data: { delay: 60 } });
    const proxies = [staticProxy()];
    const providers = [provider([providerNode()])];
    const rows = buildNodeRows(proxies, providers);
    const onTestAllDone = vi.fn();
    const props = {
      rows,
      providers,
      onEdit: vi.fn(),
      onDeleted: vi.fn(),
      onCopied: vi.fn(),
      onTestAllDone,
    };
    const { rerender } = render(<ProxyList {...props} testAllTrigger={0} />);
    rerender(<ProxyList {...props} testAllTrigger={1} />);

    await waitFor(() => expect(onTestAllDone).toHaveBeenCalledTimes(1));
    expect(proxyDelay.mock.calls.map((call) => call[0])).toEqual(['Static HK 01']);
    expect(providerNodeHealthcheck.mock.calls).toEqual([
      ['Demo Provider', 'Demo HK / 01 + #1'],
    ]);
  });

  it('uses the provided empty message instead of claiming there are no proxies', () => {
    renderList({ proxies: [], providers: [provider(null)], emptyMessage: 'Load failed / unknown' });
    expect(screen.getByText('Load failed / unknown')).toBeInTheDocument();
  });
});
