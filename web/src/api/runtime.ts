import { api } from './client';

export const runtimeApi = {
  connections: () => api.get('runtime/connections').json<{ data: any }>(),
  closeConnection: (id: string) => api.delete(`runtime/connections/${id}`).json(),
  proxies: () => api.get('runtime/proxies').json<{ data: any }>(),
  proxyDelay: (name: string, url?: string, timeout?: number) =>
    api
      .get(`runtime/proxies/${encodeURIComponent(name)}/delay`, {
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
  providers: () => api.get('runtime/providers/rules').json<{ data: any }>(),
  refreshProvider: (name: string) =>
    api.put(`runtime/providers/rules/${encodeURIComponent(name)}`).json(),
  version: () => api.get('runtime/version').json<{ data: any }>(),
};
