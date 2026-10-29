import type { Proxy } from '@/api/proxies';
import type { RuntimeProviderNode, RuntimeProxyProvider } from '@/api/runtime';

/** Source filter value that matches every node. */
export const ALL_SOURCES = '__all__';
/** Source filter value that matches static (database) nodes only. */
export const STATIC_SOURCE = '__static__';

/** Source filter value for one native proxy-provider (subscription). */
export function providerSource(name: string): string {
  return `provider:${name}`;
}

export type NodeKind = 'static' | 'provider';

/** One row of the unified node-management table.
 *
 * Static rows come from the /api/proxies CRUD list (editable, reorderable,
 * deletable). Provider rows are read-only runtime nodes of a native mihomo
 * proxy-provider; they only expose display-safe metadata.
 */
export interface NodeRow {
  /** Unique React key / delay-test state key across all sources. */
  key: string;
  kind: NodeKind;
  /** Static proxy id (UUID) or provider-scoped node id. */
  id: string;
  /** Runtime proxy name — used verbatim for delay tests. */
  name: string;
  /** Normalized protocol id, e.g. `ss` or `anytls`. */
  protocol: string;
  /** Empty for static rows. */
  providerName: string;
  providerStatus: 'ok' | 'unknown' | null;
  country: string;
  server: string;
  alive: boolean | null;
  udp: boolean | null;
  delay: number | null;
  readOnly: boolean;
  /** Static rows only. */
  proxy?: Proxy;
  /** Provider rows only. */
  providerNode?: RuntimeProviderNode;
}

const PROTOCOL_ALIASES: Record<string, string> = {
  shadowsocks: 'ss',
  shadowsocksr: 'ssr',
  ssr: 'ssr',
  vmess: 'vmess',
  vless: 'vless',
  trojan: 'trojan',
  hysteria: 'hysteria',
  hysteria2: 'hysteria2',
  tuic: 'tuic',
  anytls: 'anytls',
  http: 'http',
  socks5: 'socks5',
  snell: 'snell',
  wireguard: 'wireguard',
  ssh: 'ssh',
  mieru: 'mieru',
};

/** Protocol filter options always offered, regardless of the current list. */
export const BASE_PROTOCOLS = [
  'ss',
  'ssr',
  'vmess',
  'vless',
  'trojan',
  'hysteria',
  'hysteria2',
  'tuic',
  'anytls',
  'http',
  'socks5',
  'snell',
  'wireguard',
];

/** Normalize mihomo's display type (`AnyTLS`, `Shadowsocks`, …) and the stored
 * static type (`ss`, `vless`, …) to one filterable id. */
export function normalizeProtocol(raw: string): string {
  const key = (raw ?? '').trim().toLowerCase().replace(/[\s_-]+/g, '');
  return PROTOCOL_ALIASES[key] ?? key;
}

function serverLabel(config: Record<string, unknown>): string {
  const server = config?.server;
  if (server === undefined || server === null || server === '') return '—';
  const port = config?.port;
  if (port === undefined || port === null || port === '') return String(server);
  return `${String(server)}:${String(port)}`;
}

/** Merge static proxies and runtime provider nodes into one ordered row list.
 *
 * Static rows keep database order; provider rows follow the provider order and
 * node order reported by the backend. Keys are made unique defensively, so two
 * providers with identically named nodes (or duplicate ids) can never collide.
 */
export function buildNodeRows(
  proxies: Proxy[],
  providers: RuntimeProxyProvider[]
): NodeRow[] {
  const used = new Set<string>();
  const rows: NodeRow[] = [];
  const push = (row: NodeRow) => {
    let key = row.key;
    let suffix = 2;
    while (used.has(key)) {
      key = `${row.key}#${suffix}`;
      suffix += 1;
    }
    used.add(key);
    rows.push(key === row.key ? row : { ...row, key });
  };

  for (const proxy of proxies ?? []) {
    const config = proxy.config ?? {};
    push({
      key: `static:${proxy.id}`,
      kind: 'static',
      id: proxy.id,
      name: proxy.name,
      protocol: normalizeProtocol(proxy.type),
      providerName: '',
      providerStatus: null,
      country: proxy.country ?? '',
      server: serverLabel(config),
      alive: null,
      udp: typeof config.udp === 'boolean' ? config.udp : null,
      delay: null,
      readOnly: false,
      proxy,
    });
  }

  for (const provider of providers ?? []) {
    for (const node of provider.nodes ?? []) {
      push({
        key: `provider:${node.id}`,
        kind: 'provider',
        id: node.id,
        name: node.name,
        protocol: normalizeProtocol(node.type),
        providerName: provider.name,
        providerStatus: provider.status,
        country: '',
        server: '—',
        alive: node.alive ?? null,
        udp: node.udp ?? null,
        delay: node.delay ?? null,
        readOnly: true,
        providerNode: node,
      });
    }
  }

  return rows;
}

export interface NodeFilters {
  search?: string;
  protocol?: string;
  source?: string;
  country?: string;
}

/** Apply the page filters to the merged rows.
 *
 * Country is a static-only attribute, so an active country filter hides
 * provider rows (they carry no country metadata). Source `provider:<name>`
 * values are compared against the provider name, never against the node name.
 */
export function filterNodeRows(rows: NodeRow[], filters: NodeFilters): NodeRow[] {
  const needle = (filters.search ?? '').trim().toLowerCase();
  const source = filters.source ?? ALL_SOURCES;
  return rows.filter((row) => {
    if (needle) {
      const haystack = `${row.name}\n${row.providerName}`.toLowerCase();
      if (!haystack.includes(needle)) return false;
    }
    if (filters.protocol && row.protocol !== filters.protocol) return false;
    if (source === STATIC_SOURCE) {
      if (row.kind !== 'static') return false;
    } else if (source.startsWith('provider:')) {
      if (row.providerName !== source.slice('provider:'.length)) return false;
    }
    if (filters.country && (row.kind !== 'static' || row.country !== filters.country)) {
      return false;
    }
    return true;
  });
}

const PROTOCOL_LABELS: Record<string, string> = {
  ss: 'SS',
  ssr: 'SSR',
  vmess: 'VMess',
  vless: 'VLESS',
  trojan: 'Trojan',
  hysteria: 'Hysteria',
  hysteria2: 'Hysteria2',
  tuic: 'TUIC',
  anytls: 'AnyTLS',
  http: 'HTTP',
  socks5: 'SOCKS5',
  snell: 'Snell',
  wireguard: 'WireGuard',
  ssh: 'SSH',
  mieru: 'Mieru',
};

/** Display label for a normalized protocol id (badges and filter options). */
export function protocolLabel(protocol: string): string {
  return PROTOCOL_LABELS[protocol] ?? protocol;
}

/** Protocol filter options: the stable base list plus any extra protocol that
 * actually appears in the current rows. */
export function protocolOptions(rows: NodeRow[]): string[] {
  const set = new Set<string>(BASE_PROTOCOLS);
  for (const row of rows) {
    if (row.protocol) set.add(row.protocol);
  }
  return [...set];
}

/** Country filter options derived from static rows only. */
export function countryOptions(rows: NodeRow[]): string[] {
  return [...new Set(rows.filter((row) => row.kind === 'static').map((row) => row.country).filter(Boolean))].sort();
}

export interface ProviderSummary {
  name: string;
  status: 'ok' | 'unknown';
  /** null = unknown (load failed / not loaded), never a fake 0. */
  nodeCount: number | null;
  updatedAt: string;
}

export function providerSummaries(providers: RuntimeProxyProvider[]): ProviderSummary[] {
  return (providers ?? []).map((provider) => ({
    name: provider.name,
    status: provider.status,
    // A provider with known nodes always has a count; otherwise stay unknown.
    nodeCount:
      provider.status === 'ok' && typeof provider.node_count === 'number'
        ? provider.node_count
        : null,
    updatedAt: provider.updated_at ?? '',
  }));
}
