import ky from 'ky';

const getToken = () => {
  try {
    const stored = localStorage.getItem('auth-storage');
    if (stored) {
      const parsed = JSON.parse(stored);
      return parsed?.state?.accessToken || null;
    }
  } catch {
    return null;
  }
  return null;
};

export const api = ky.create({
  prefixUrl: '/api',
  hooks: {
    beforeRequest: [
      (request) => {
        const token = getToken();
        if (token) {
          request.headers.set('Authorization', `Bearer ${token}`);
        }
      },
    ],
    afterResponse: [
      async (_request, _options, response) => {
        if (response.status === 401) {
          localStorage.removeItem('auth-storage');
          if (window.location.pathname !== '/login') {
            window.location.href = '/login';
          }
        }
      },
    ],
  },
});
