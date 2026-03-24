import { api } from './client';

export const settingsApi = {
  get: () => api.get('settings').json<{ data: any }>(),
  update: (data: any) => api.put('settings', { json: data }).json(),
  listUsers: () => api.get('settings/users').json<{ data: any[] }>(),
  createUser: (data: { username: string; password: string; role: string }) =>
    api.post('settings/users', { json: data }).json(),
  updateUser: (id: string, data: any) => api.put(`settings/users/${id}`, { json: data }).json(),
  deleteUser: (id: string) => api.delete(`settings/users/${id}`).json(),
  getConfigYaml: () => api.get('settings/config-yaml').json<{ data: { content: string; source: string } }>(),
  updateConfigYaml: (content: string) =>
    api.put('settings/config-yaml', { json: { content } }).json(),
  deleteConfigYaml: () => api.delete('settings/config-yaml').json(),
};
