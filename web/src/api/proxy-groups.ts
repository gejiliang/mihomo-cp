import { api } from './client';

export interface ProxyGroup {
  id: string;
  name: string;
  type: string; // select, fallback, url-test, load-balance, relay
  config: Record<string, any>;
  members: string[]; // ordered list of proxy/group names
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export const proxyGroupsApi = {
  list: (params?: { search?: string; type?: string }) =>
    api.get('proxy-groups', { searchParams: params as any }).json<{ data: ProxyGroup[] }>(),
  get: (id: string) => api.get(`proxy-groups/${id}`).json<{ data: ProxyGroup }>(),
  create: (data: Partial<ProxyGroup>) =>
    api.post('proxy-groups', { json: data }).json<{ data: ProxyGroup }>(),
  update: (id: string, data: Partial<ProxyGroup>) =>
    api.put(`proxy-groups/${id}`, { json: data }).json<{ data: ProxyGroup }>(),
  delete: (id: string) => api.delete(`proxy-groups/${id}`).json(),
  reorder: (ids: string[]) => api.post('proxy-groups/reorder', { json: { ids } }).json(),
  refs: (id: string) => api.get(`proxy-groups/${id}/refs`).json<{ data: string[] }>(),
};
