/**
 * Presentation helpers for subscription quota, reset schedules and masked URLs.
 *
 * The API deliberately returns `null` for values it does not know. These helpers
 * keep that distinction: unknown is rendered as the localized "unknown" label,
 * never as 0 B or unlimited.
 */

const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

/** Formats a byte count, or returns null when the value is unknown. */
export function formatBytesExact(bytes: number | null | undefined): string | null {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return null;
  if (bytes === 0) return '0 B';
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNITS.length - 1);
  const value = bytes / 1024 ** i;
  const digits = i === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${UNITS[i]}`;
}

/** Formats a GiB quantity with up to two decimals, e.g. 200 GiB. */
export function formatGiB(bytes: number | null | undefined): string | null {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return null;
  const gib = bytes / 1024 ** 3;
  const digits = Number.isInteger(gib) ? 0 : gib >= 100 ? 1 : 2;
  return `${gib.toFixed(digits)} GiB`;
}

export function formatPercent(
  used: number | null | undefined,
  total: number | null | undefined
): number | null {
  if (used === null || used === undefined || total === null || total === undefined) return null;
  if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return null;
  return Math.min(Math.max((used / total) * 100, 0), 100);
}

export const DEFAULT_LOCALE: Record<'en' | 'zh', string> = {
  en: 'en-GB',
  zh: 'zh-CN',
};

/**
 * Formats an ISO timestamp in the given IANA timezone (or browser timezone when
 * omitted). Returns null for missing/invalid input.
 */
export function formatDateTime(
  iso: string | null | undefined,
  locale: string,
  timeZone?: string | null
): string | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  try {
    return new Intl.DateTimeFormat(locale, {
      timeZone: timeZone || undefined,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(date);
  } catch {
    return null;
  }
}

/**
 * Formats a date/time together with its timezone, e.g. "2026-11-10 12:00 (Asia/Shanghai)".
 */
export function formatDateTimeWithZone(
  iso: string | null | undefined,
  locale: string,
  timeZone?: string | null
): string | null {
  const formatted = formatDateTime(iso, locale, timeZone);
  if (!formatted) return null;
  return timeZone ? `${formatted} (${timeZone})` : formatted;
}

function parseOffsetMs(offset: string): number | null {
  if (offset === 'Z' || offset === 'z') return 0;
  const match = /^([+-])(\d{2}):?(\d{2})$/.exec(offset);
  if (!match) return null;
  const minutes = Number(match[2]) * 60 + Number(match[3]);
  return (match[1] === '+' ? 1 : -1) * minutes * 60_000;
}

function offsetLabel(offset: string): string {
  if (offset === 'Z' || offset === 'z') return 'UTC';
  return `UTC${offset}`;
}

/**
 * Formats an API timestamp that may carry its own RFC3339 offset. When the
 * caller knows the IANA zone it wins; otherwise an explicit offset in the
 * string is preserved instead of silently converting to the browser zone
 * (the backend may run in a different server-local timezone).
 */
export function formatApiDateTime(
  iso: string | null | undefined,
  locale: string,
  timeZone?: string | null
): string | null {
  if (!iso) return null;
  if (timeZone) return formatDateTimeWithZone(iso, locale, timeZone);
  const match = /(Z|[+-]\d{2}:?\d{2})$/.exec(iso);
  const offsetMs = match ? parseOffsetMs(match[1]) : null;
  if (!match || offsetMs === null) return formatDateTime(iso, locale);
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  const shifted = new Date(date.getTime() + offsetMs);
  try {
    const wall = new Intl.DateTimeFormat(locale, {
      timeZone: 'UTC',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(shifted);
    return `${wall} (${offsetLabel(match[1])})`;
  } catch {
    return null;
  }
}

/**
 * Masks a subscription URL so tokens are never rendered in lists or logs.
 * Keeps only scheme and host, e.g. "https://example.com/••••".
 */
export function maskUrl(url: string | null | undefined): string | null {
  if (!url) return null;
  try {
    const parsed = new URL(url);
    return `${parsed.protocol}//${parsed.host}/••••`;
  } catch {
    return '••••';
  }
}

/** Returns the display value for a masked URL, preferring the server-side mask. */
export function displayMaskedUrl(
  masked: string | null | undefined,
  full: string | null | undefined
): string | null {
  if (masked) return masked;
  return maskUrl(full);
}

/** "Reset in X days" using whole days rounded up; null when unknown. */
export function daysUntil(iso: string | null | undefined, now: Date = new Date()): number | null {
  if (!iso) return null;
  const target = new Date(iso);
  if (Number.isNaN(target.getTime())) return null;
  const ms = target.getTime() - now.getTime();
  if (ms <= 0) return 0;
  return Math.ceil(ms / 86_400_000);
}

/** Values that are stale (last successful refresh older than threshold) are flagged. */
export function isStale(
  lastSuccessAt: string | null | undefined,
  staleAfterMinutes = 24 * 60,
  now: Date = new Date()
): boolean {
  if (!lastSuccessAt) return false;
  const date = new Date(lastSuccessAt);
  if (Number.isNaN(date.getTime())) return false;
  return now.getTime() - date.getTime() > staleAfterMinutes * 60_000;
}
