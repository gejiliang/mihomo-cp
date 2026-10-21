import { HTTPError } from 'ky';

export interface ApiErrorInfo {
  status: number | null;
  code: string;
  message: string;
  references: string[] | null;
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function readReferences(value: unknown): string[] | null {
  if (!Array.isArray(value)) return null;
  const refs = value.filter((v): v is string => typeof v === 'string');
  return refs;
}

/**
 * Extracts the standard API error envelope without ever echoing request
 * bodies (which may contain subscription URLs). Server messages are already
 * sanitized by the backend.
 */
export async function readApiError(error: unknown): Promise<ApiErrorInfo> {
  if (!(error instanceof HTTPError)) {
    return { status: null, code: '', message: '', references: null };
  }
  let body: unknown = null;
  try {
    body = await error.response.clone().json();
  } catch {
    body = null;
  }
  const root = (body && typeof body === 'object' ? body : {}) as Record<string, unknown>;
  const envelope = (root.error && typeof root.error === 'object' ? root.error : {}) as Record<
    string,
    unknown
  >;
  const data = (root.data && typeof root.data === 'object' ? root.data : {}) as Record<
    string,
    unknown
  >;
  return {
    status: error.response.status,
    code: readString(envelope.code) || readString(root.code),
    message: readString(envelope.message) || readString(root.message),
    references:
      readReferences(envelope.references) ??
      readReferences(data.references) ??
      readReferences(root.references),
  };
}
