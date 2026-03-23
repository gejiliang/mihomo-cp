import { api } from './client';

export interface LoginResponse {
  data: {
    access_token: string;
    refresh_token: string;
  };
}

export interface MeResponse {
  data: {
    id: string;
    username: string;
    role: string;
  };
}

export const authApi = {
  login: (username: string, password: string) =>
    api.post('auth/login', { json: { username, password } }).json<LoginResponse>(),
  refresh: (refreshToken: string) =>
    api.post('auth/refresh', { json: { refresh_token: refreshToken } }).json(),
  me: () => api.get('auth/me').json<MeResponse>(),
};
