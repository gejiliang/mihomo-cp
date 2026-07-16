import { api } from './client';

export interface Proxy {
  id: string;
  name: string;
  type: string;
  country: string;
  config: Record<string, unknown>;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export const proxiesApi = {
  list: (params?: { search?: string; type?: string }) =>
    api.get('proxies', { searchParams: params as Record<string, string> }).json<{ data: Proxy[] }>(),
  get: (id: string) => api.get(`proxies/${id}`).json<{ data: Proxy }>(),
  create: (data: Partial<Proxy>) => api.post('proxies', { json: data }).json<{ data: Proxy }>(),
  update: (id: string, data: Partial<Proxy>) => api.put(`proxies/${id}`, { json: data }).json<{ data: Proxy }>(),
  delete: (id: string) => api.delete(`proxies/${id}`).json(),
  copy: (id: string) => api.post(`proxies/${id}/copy`).json<{ data: Proxy }>(),
  reorder: (ids: string[]) => api.post('proxies/reorder', { json: { ids } }).json(),
  refs: (id: string) => api.get(`proxies/${id}/refs`).json<{ data: string[] }>(),
  detectCountries: (countries?: Record<string, string>) =>
    api.post('proxies/detect-countries', { json: countries ? { countries } : {}, timeout: 120_000 }).json<{ data: { total: number; updated: number } }>(),
};
