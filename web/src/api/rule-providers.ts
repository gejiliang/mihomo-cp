import { api } from './client';

export interface RuleProvider {
  id: string;
  name: string;
  type: string; // file, http
  behavior: string; // domain, ipcidr, classical
  config: Record<string, any>;
}

export const ruleProvidersApi = {
  list: () => api.get('rule-providers').json<{ data: RuleProvider[] }>(),
  create: (data: Partial<RuleProvider>) => api.post('rule-providers', { json: data }).json(),
  update: (id: string, data: Partial<RuleProvider>) =>
    api.put(`rule-providers/${id}`, { json: data }).json(),
  delete: (id: string) => api.delete(`rule-providers/${id}`).json(),
  refresh: (id: string) => api.post(`rule-providers/${id}/refresh`).json(),
};
