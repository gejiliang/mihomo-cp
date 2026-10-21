import { api } from './client';

/** Vendor snapshot from the subscription-userinfo response header. */
export interface SubscriptionUserinfo {
  upload: number;
  download: number;
  total: number;
  expire: number; // unix seconds, 0 = unknown
  fetched_at: string;
}

export interface Subscription {
  id: string;
  name: string;
  enabled: boolean;
  status: 'ok' | 'stale' | 'error' | 'disabled';
  last_error: string;
  provider_name: string;
  provider_source: 'created' | 'linked' | string;
  provider_exists: boolean;
  url_masked: string;
  node_count: number | null;
  quota_bytes: number | null;
  used_bytes: number | null;
  remaining_bytes: number | null;
  usage_source: 'vendor' | 'local' | 'manual' | 'none' | string;
  local_used_bytes: number | null;
  expires_at: string | null;
  expires_source: 'vendor' | 'manual' | 'unknown' | string;
  reset_day: number;
  reset_hour: number;
  reset_minute: number;
  timezone: string | null;
  cycle_start: string | null;
  cycle_end: string | null;
  next_reset: string | null;
  userinfo_fetched_at: string | null;
  last_refresh_at: string | null;
  last_success_at: string | null;
  stale: boolean;
  userinfo: SubscriptionUserinfo | null;
  traffic_provider_id: string;
  created_at: string;
  updated_at: string;
  draft_changed?: boolean;
}

export interface SubscriptionInput {
  name: string;
  url?: string;
  provider_name?: string;
  quota_bytes?: number | null;
  manual_expire?: string | null;
  reset_day: number;
  reset_hour: number;
  reset_minute: number;
  timezone?: string;
  traffic_provider_id?: string;
  enabled: boolean;
}

export interface DiscoveredProvider {
  name: string;
  type: string;
  node_count: number | null;
  linked_subscription_id: string | null;
  linked: boolean;
}

export interface SubscriptionRefreshError {
  code: 'rate_limited' | 'userinfo_failed' | 'provider_refresh_failed' | 'not_found' | string;
  message: string;
  retry_after_seconds?: number;
}

export interface SubscriptionRefreshResult {
  subscription: Subscription;
  error: SubscriptionRefreshError | null;
}

export interface DeleteSubscriptionResult {
  status: string;
  provider_name: string;
  provider_removed_from_draft?: boolean;
  draft_changed?: boolean;
}

/** Treats Go zero timestamps and empty strings as unknown. */
export function normalizeTimestamp(value: string | null | undefined): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  if (date.getUTCFullYear() <= 1) return null;
  return value;
}

export const subscriptionsApi = {
  list: () => api.get('subscriptions').json<{ data: Subscription[] }>(),
  get: (id: string) => api.get(`subscriptions/${id}`).json<{ data: Subscription }>(),
  providers: () => api.get('subscriptions/providers').json<{ data: DiscoveredProvider[] }>(),
  create: (input: SubscriptionInput) =>
    api.post('subscriptions', { json: input }).json<{ data: Subscription }>(),
  update: (id: string, input: SubscriptionInput) =>
    api.put(`subscriptions/${id}`, { json: input }).json<{ data: Subscription }>(),
  remove: (id: string) =>
    api.delete(`subscriptions/${id}`).json<{ data: DeleteSubscriptionResult }>(),
  refresh: (id: string) =>
    api
      .post(`subscriptions/${id}/refresh`, { timeout: 120_000 })
      .json<{ data: SubscriptionRefreshResult }>(),
};
