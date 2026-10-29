import { describe, expect, it } from 'vitest';
import type { Proxy } from '@/api/proxies';
import type { RuntimeProxyProvider } from '@/api/runtime';
import {
  ALL_SOURCES,
  STATIC_SOURCE,
  buildNodeRows,
  countryOptions,
  filterNodeRows,
  normalizeProtocol,
  protocolLabel,
  protocolOptions,
  providerSource,
  providerSummaries,
} from './node-inventory';

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

function provider(
  name: string,
  nodes: RuntimeProxyProvider['nodes'],
  status: 'ok' | 'unknown' = 'ok'
): RuntimeProxyProvider {
  return {
    name,
    type: 'Proxy',
    vehicle_type: 'HTTP',
    status,
    updated_at: status === 'ok' ? '2026-10-20T08:00:00Z' : '',
    node_count: status === 'ok' ? (nodes?.length ?? 0) : null,
    nodes,
  };
}

const PROVIDERS: RuntimeProxyProvider[] = [
  provider('Demo Provider', [
    {
      id: 'provider/Demo%20Provider/Demo%20HK%20%2F%2001%20%2B%20%231',
      name: 'Demo HK / 01 + #1',
      type: 'AnyTLS',
      provider: 'Demo Provider',
      alive: true,
      udp: true,
      delay: 120,
    },
    {
      id: 'provider/Demo%20Provider/Shared%20Node',
      name: 'Shared Node',
      type: 'Shadowsocks',
      provider: 'Demo Provider',
      alive: false,
      udp: null,
      delay: null,
    },
  ]),
  provider('Second Provider', [
    {
      id: 'provider/Second%20Provider/Shared%20Node',
      name: 'Shared Node',
      type: 'VLESS',
      provider: 'Second Provider',
      alive: null,
      udp: null,
      delay: null,
    },
  ]),
  provider('Broken Provider', null, 'unknown'),
];

describe('normalizeProtocol', () => {
  it('maps mihomo display types and stored ids to one filterable id', () => {
    expect(normalizeProtocol('AnyTLS')).toBe('anytls');
    expect(normalizeProtocol('Shadowsocks')).toBe('ss');
    expect(normalizeProtocol('VLESS')).toBe('vless');
    expect(normalizeProtocol('Hysteria2')).toBe('hysteria2');
    expect(normalizeProtocol('SOCKS5')).toBe('socks5');
    expect(normalizeProtocol('ss')).toBe('ss');
    expect(normalizeProtocol('')).toBe('');
  });

  it('renders readable labels for badges and filter options', () => {
    expect(protocolLabel('anytls')).toBe('AnyTLS');
    expect(protocolLabel('ss')).toBe('SS');
    expect(protocolLabel('custom-proto')).toBe('custom-proto');
  });
});

describe('buildNodeRows', () => {
  it('lists static and provider nodes together with clear sources and read-only flags', () => {
    const rows = buildNodeRows([staticProxy()], PROVIDERS);

    expect(rows.map((row) => row.name)).toEqual([
      'Static HK 01',
      'Demo HK / 01 + #1',
      'Shared Node',
      'Shared Node',
    ]);
    expect(rows[0]).toMatchObject({
      kind: 'static',
      readOnly: false,
      protocol: 'ss',
      country: 'HK',
      server: '198.51.100.10:8388',
      udp: true,
    });
    expect(rows[1]).toMatchObject({
      kind: 'provider',
      readOnly: true,
      protocol: 'anytls',
      providerName: 'Demo Provider',
      providerStatus: 'ok',
      alive: true,
      udp: true,
      delay: 120,
    });
    // Provider nodes never expose a server address.
    expect(rows[1].server).toBe('—');
  });

  it('keeps React keys unique for same-named nodes and special characters', () => {
    const rows = buildNodeRows([staticProxy(), staticProxy({ id: 'same-name', name: 'Shared Node' })], PROVIDERS);
    const keys = rows.map((row) => row.key);
    expect(new Set(keys).size).toBe(keys.length);
    // Same node name in two providers produces two distinct, provider-scoped keys.
    const sharedKeys = rows.filter((row) => row.name === 'Shared Node').map((row) => row.key);
    expect(sharedKeys).toHaveLength(3);
    expect(new Set(sharedKeys).size).toBe(3);
    expect(keys.every((key) => !key.includes(' '))).toBe(true);
  });

  it('defensively disambiguates duplicate keys from a misbehaving backend', () => {
    const duplicateId = 'provider/P1/Same';
    const rows = buildNodeRows(
      [],
      [
        provider('P1', [
          { id: duplicateId, name: 'Same', type: 'AnyTLS', provider: 'P1', alive: null, udp: null, delay: null },
        ]),
        provider('P2', [
          { id: duplicateId, name: 'Same', type: 'AnyTLS', provider: 'P2', alive: null, udp: null, delay: null },
        ]),
      ]
    );
    expect(rows.map((row) => row.key)).toEqual(['provider:provider/P1/Same', 'provider:provider/P1/Same#2']);
  });

  it('does not create rows for a provider with an unknown node list', () => {
    const rows = buildNodeRows([], [provider('Broken Provider', null, 'unknown')]);
    expect(rows).toEqual([]);
  });
});

describe('filterNodeRows', () => {
  const rows = buildNodeRows([staticProxy()], PROVIDERS);

  it('searches static and provider node names, including provider names', () => {
    expect(filterNodeRows(rows, { search: 'demo hk' }).map((row) => row.name)).toEqual([
      'Demo HK / 01 + #1',
    ]);
    expect(filterNodeRows(rows, { search: 'demo provider' })).toHaveLength(2);
    expect(filterNodeRows(rows, { search: '  01 + #1 ' })).toHaveLength(1);
  });

  it('filters by protocol, including AnyTLS subscription nodes', () => {
    expect(filterNodeRows(rows, { protocol: 'anytls' }).map((row) => row.name)).toEqual([
      'Demo HK / 01 + #1',
    ]);
    expect(filterNodeRows(rows, { protocol: 'ss' }).map((row) => row.name)).toEqual([
      'Static HK 01',
      'Shared Node',
    ]);
  });

  it('filters by source: all, static, or one provider name', () => {
    expect(filterNodeRows(rows, { source: ALL_SOURCES })).toHaveLength(rows.length);
    expect(filterNodeRows(rows, { source: STATIC_SOURCE }).map((row) => row.kind)).toEqual(['static']);
    expect(
      filterNodeRows(rows, { source: providerSource('Second Provider') }).map((row) => row.providerName)
    ).toEqual(['Second Provider']);
    // A provider named like a filter keyword cannot alias the static filter.
    const keywordProvider = provider('static', [
      { id: 'provider/static/Node', name: 'Static Named Node', type: 'AnyTLS', provider: 'static', alive: null, udp: null, delay: null },
    ]);
    const keywordRows = buildNodeRows([staticProxy()], [keywordProvider]);
    expect(
      filterNodeRows(keywordRows, { source: providerSource('static') }).map((row) => row.name)
    ).toEqual(['Static Named Node']);
  });

  it('treats country as a static-only attribute', () => {
    expect(filterNodeRows(rows, { country: 'HK' }).map((row) => row.name)).toEqual(['Static HK 01']);
    expect(filterNodeRows(rows, { country: 'JP' })).toEqual([]);
  });
});

describe('filter options and provider summaries', () => {
  it('always offers AnyTLS and adds protocols found in the rows', () => {
    expect(protocolOptions([])).toContain('anytls');
    expect(protocolOptions([])).toContain('hysteria2');
    const rows = buildNodeRows(
      [],
      [provider('P1', [{ id: 'provider/P1/M', name: 'M', type: 'Mieru', provider: 'P1', alive: null, udp: null, delay: null }])]
    );
    expect(protocolOptions(rows)).toContain('mieru');
  });

  it('derives country options from static rows only', () => {
    const rows = buildNodeRows(
      [staticProxy({ country: 'JP' }), staticProxy({ id: 'b', country: 'HK' }), staticProxy({ id: 'c', country: '' })],
      PROVIDERS
    );
    expect(countryOptions(rows)).toEqual(['HK', 'JP']);
  });

  it('keeps an unknown provider count unknown instead of showing 0', () => {
    const summaries = providerSummaries([provider('Broken Provider', null, 'unknown'), provider('Empty Provider', [])]);
    expect(summaries).toEqual([
      { name: 'Broken Provider', status: 'unknown', nodeCount: null, updatedAt: '' },
      { name: 'Empty Provider', status: 'ok', nodeCount: 0, updatedAt: '2026-10-20T08:00:00Z' },
    ]);
  });
});
