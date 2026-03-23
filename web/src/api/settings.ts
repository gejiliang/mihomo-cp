import { api } from './client';

export const settingsApi = {
  get: () => api.get('settings').json<{ data: any }>(),
  update: (data: any) => api.put('settings', { json: data }).json(),
  listUsers: () => api.get('users').json<{ data: any[] }>(),
  createUser: (data: { username: string; password: string; role: string }) =>
    api.post('users', { json: data }).json(),
  updateUser: (id: string, data: any) => api.put(`users/${id}`, { json: data }).json(),
  deleteUser: (id: string) => api.delete(`users/${id}`).json(),
};
