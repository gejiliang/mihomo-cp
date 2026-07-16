import { api } from './client';

export interface Rule {
  id: string;
  type: string;
  payload: string;
  target: string;
  params: Record<string, any>;
  sort_order: number;
}

export const rulesApi = {
  list: (params?: { search?: string; type?: string }) =>
    api.get('rules', { searchParams: params as any }).json<{ data: Rule[] }>(),
  create: (data: Partial<Rule>) => api.post('rules', { json: data }).json<{ data: Rule }>(),
  update: (id: string, data: Partial<Rule>) =>
    api.put(`rules/${id}`, { json: data }).json<{ data: Rule }>(),
  delete: (id: string) => api.delete(`rules/${id}`).json(),
  reorder: (ids: string[]) => api.post('rules/reorder', { json: { ids } }).json(),
};
