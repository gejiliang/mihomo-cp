import { api } from './client';
import { proxyDelayPath } from '@/lib/proxy-path';

/** One read-only node of a native mihomo proxy-provider (subscription).
 *
 * Only display-safe metadata: the backend never sends the provider URL, the
 * server address or any node credential. */
export interface RuntimeProviderNode {
  /** Provider-scoped stable id, e.g. `provider/<escaped provider>/<escaped name>`. */
  id: string;
  /** Real runtime node name — used verbatim for delay tests. */
  name: string;
  type: string;
  provider: string;
  alive: boolean | null;
  udp: boolean | null;
  delay: number | null;
}

/** One native proxy-provider as reported by the running mihomo. */
export interface RuntimeProxyProvider {
  name: string;
  type: string;
  vehicle_type: string;
  /** `ok` = node list loaded; `unknown` = provider did not report nodes. */
  status: 'ok' | 'unknown';
  updated_at: string;
  /** null when the node list is unknown — never a fake 0. */
  node_count: number | null;
  nodes: RuntimeProviderNode[] | null;
}

export const runtimeApi = {
  connections: () => api.get('runtime/connections').json<{ data: any }>(),
  closeConnection: (id: string) => api.delete(`runtime/connections/${id}`).json(),
  proxies: () => api.get('runtime/proxies').json<{ data: any }>(),
  proxyDelay: (name: string, url?: string, timeout?: number) =>
    api
      .get(proxyDelayPath(name), {
        searchParams: {
          url: url || 'http://www.gstatic.com/generate_204',
          timeout: timeout || 5000,
        },
      })
      .json<{ data: any }>(),
  switchProxy: (group: string, proxy: string) =>
    api
      .put(`runtime/proxies/${encodeURIComponent(group)}/selected`, { json: { name: proxy } })
      .json(),
  rules: () => api.get('runtime/rules').json<{ data: any }>(),
  providers: () => api.get('runtime/providers').json<{ data: any }>(),
  proxyProviders: () =>
    api.get('runtime/proxy-providers').json<{ data: { providers: RuntimeProxyProvider[] } }>(),
  refreshProvider: (name: string) =>
    api.put(`runtime/providers/rules/${encodeURIComponent(name)}`).json(),
  version: () => api.get('runtime/version').json<{ data: any }>(),
};
