import { api } from './client';

export interface PublishRecord {
  id: string;
  version: number;
  config_yaml: string;
  diff_text: string;
  status: string; // success, failed, rolled_back
  error_msg: string;
  operator: string;
  note: string;
  created_at: string;
}

export interface PublishStatus {
  has_changes: boolean;
  running_version: number;
}

export interface PublishPreview {
  yaml: string;
  diff: string;
}

export interface ValidationResult {
  valid: boolean;
  output: string;
  errors: Array<{ code: string; level: string; message: string }>;
}

export const publishApi = {
  preview: () => api.get('publish/preview').json<{ data: PublishPreview }>(),
  validate: () => api.post('publish/validate').json<{ data: ValidationResult }>(),
  publish: (note?: string) =>
    api.post('publish', { json: { note } }).json<{ data: PublishRecord }>(),
  rollback: () => api.post('publish/rollback').json<{ data: PublishRecord }>(),
  history: (limit?: number) =>
    api
      .get('publish/history', { searchParams: { limit: limit || 20 } })
      .json<{ data: PublishRecord[] }>(),
  historyDetail: (id: string) =>
    api.get(`publish/history/${id}`).json<{ data: PublishRecord }>(),
  status: () => api.get('publish/status').json<{ data: PublishStatus }>(),
};
