import { api } from './client';

export const systemConfigApi = {
  get: () => api.get('system-config').json<{ data: Record<string, any> }>(),
  update: (config: Record<string, any>) =>
    api.put('system-config', { json: config }).json(),
};
