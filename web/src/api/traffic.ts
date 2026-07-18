import { api } from './client';

export interface TrafficProvider {
  id: string;
  name: string;
  quota_bytes: number;
  reset_day: number;
  node_patterns: string[];
  subscription_url: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface TrafficUserinfo {
  provider_id: string;
  upload: number;
  download: number;
  total: number;
  expire: number;
  fetched_at: string;
}

export interface NodeUsage {
  node: string;
  upload: number;
  download: number;
  total: number;
}

export interface ProviderSummary {
  provider: TrafficProvider;
  cycle_start: string;
  cycle_end: string;
  local: { upload: number; download: number; total: number };
  reported?: TrafficUserinfo;
  usage_bytes: number;
  percent: number;
  today_bytes: number;
  est_days_left: number;
  top_nodes: NodeUsage[];
}

export interface TrafficSummary {
  providers: ProviderSummary[];
  unassigned: NodeUsage[];
}

export interface DailyUsage {
  date: string;
  providers: Record<string, number>;
  unassigned: number;
}

export interface TrafficAlert {
  id: string;
  provider_id: string;
  cycle_start: string;
  threshold: number;
  usage_bytes: number;
  quota_bytes: number;
  message: string;
  notified: boolean;
  created_at: string;
}

export interface TrafficSettings {
  id: number;
  alert_channel: string;
  alert_target: string;
  alert_thresholds: string;
  updated_at: string;
}

export interface ProviderInput {
  name: string;
  quota_bytes: number;
  reset_day: number;
  node_patterns: string[];
  subscription_url: string;
  enabled: boolean;
}

export const trafficApi = {
  summary: () => api.get('traffic/summary').json<{ data: TrafficSummary }>(),
  history: (days = 30) =>
    api.get('traffic/history', { searchParams: { days } }).json<{ data: DailyUsage[] }>(),
  providers: () => api.get('traffic/providers').json<{ data: TrafficProvider[] }>(),
  createProvider: (p: ProviderInput) =>
    api.post('traffic/providers', { json: p }).json<{ data: TrafficProvider }>(),
  updateProvider: (id: string, p: ProviderInput) =>
    api.put(`traffic/providers/${id}`, { json: p }).json<{ data: TrafficProvider }>(),
  deleteProvider: (id: string) => api.delete(`traffic/providers/${id}`).json(),
  alerts: (limit = 50) =>
    api.get('traffic/alerts', { searchParams: { limit } }).json<{ data: TrafficAlert[] }>(),
  settings: () => api.get('traffic/settings').json<{ data: TrafficSettings }>(),
  updateSettings: (s: Partial<TrafficSettings>) =>
    api.put('traffic/settings', { json: s }).json<{ data: TrafficSettings }>(),
  testNotify: () => api.post('traffic/test-notify').json(),
};

export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const v = bytes / 1024 ** i;
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(v >= 10 ? 1 : 2)} ${units[i]}`;
}

export const GB = 1024 ** 3;
