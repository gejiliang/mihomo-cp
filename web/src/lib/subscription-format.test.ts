import { describe, expect, it } from 'vitest';
import { normalizeTimestamp } from '@/api/subscriptions';
import {
  daysUntil,
  displayMaskedUrl,
  formatApiDateTime,
  formatBytesExact,
  formatDateTimeWithZone,
  formatGiB,
  formatPercent,
  isStale,
  maskUrl,
} from './subscription-format';

describe('formatBytesExact', () => {
  it('keeps unknown distinct from zero', () => {
    expect(formatBytesExact(null)).toBeNull();
    expect(formatBytesExact(undefined)).toBeNull();
    expect(formatBytesExact(0)).toBe('0 B');
  });

  it('formats binary units', () => {
    expect(formatBytesExact(200 * 1024 ** 3)).toBe('200 GiB');
    expect(formatBytesExact(1536)).toBe('1.50 KiB');
  });

  it('rejects negative and non-finite values instead of faking a quota', () => {
    expect(formatBytesExact(-1)).toBeNull();
    expect(formatBytesExact(Number.NaN)).toBeNull();
    expect(formatBytesExact(Number.POSITIVE_INFINITY)).toBeNull();
  });
});

describe('formatGiB', () => {
  it('formats 200 GiB example quota', () => {
    expect(formatGiB(200 * 1024 ** 3)).toBe('200 GiB');
  });
});

describe('formatPercent', () => {
  it('clamps and returns null when either side is unknown', () => {
    expect(formatPercent(50, 200)).toBe(25);
    expect(formatPercent(500, 200)).toBe(100);
    expect(formatPercent(null, 200)).toBeNull();
    expect(formatPercent(50, null)).toBeNull();
    expect(formatPercent(50, 0)).toBeNull();
  });
});

describe('maskUrl', () => {
  it('masks path, query and credentials', () => {
    expect(maskUrl('https://example.com/sub?token=secret')).toBe('https://example.com/••••');
    expect(maskUrl('https://user:pass@example.com/sub')).toBe('https://example.com/••••');
  });

  it('never returns the original string when parsing fails', () => {
    expect(maskUrl('secret-token')).toBe('••••');
    expect(maskUrl(null)).toBeNull();
  });

  it('prefers a server-side mask', () => {
    expect(displayMaskedUrl('https://example.com/…', 'https://example.com/real')).toBe(
      'https://example.com/…'
    );
    expect(displayMaskedUrl(null, 'https://example.com/real')).toBe('https://example.com/••••');
  });
});

describe('formatDateTimeWithZone', () => {
  it('renders the reset moment in the schedule timezone', () => {
    const formatted = formatDateTimeWithZone(
      '2026-11-10T04:00:00.000Z',
      'en-GB',
      'Asia/Shanghai'
    );
    expect(formatted).toBe('10/11/2026, 12:00 (Asia/Shanghai)');
  });

  it('returns null for missing or invalid timestamps', () => {
    expect(formatDateTimeWithZone(null, 'en-GB', 'UTC')).toBeNull();
    expect(formatDateTimeWithZone('not-a-date', 'en-GB', 'UTC')).toBeNull();
  });

  it('handles month-end boundaries through the browser timezone database', () => {
    expect(formatDateTimeWithZone('2026-03-01T03:59:59Z', 'en-GB', 'Asia/Shanghai')).toBe(
      '01/03/2026, 11:59 (Asia/Shanghai)'
    );
    expect(formatDateTimeWithZone('2026-03-01T04:00:00Z', 'en-GB', 'Asia/Shanghai')).toBe(
      '01/03/2026, 12:00 (Asia/Shanghai)'
    );
  });
});

describe('daysUntil', () => {
  it('rounds up partial days and clamps past dates to zero', () => {
    const now = new Date('2026-11-01T00:00:00Z');
    expect(daysUntil('2026-11-10T04:00:00.000Z', now)).toBe(10);
    expect(daysUntil('2026-10-01T00:00:00Z', now)).toBe(0);
    expect(daysUntil(null, now)).toBeNull();
  });
});

describe('isStale', () => {
  it('only flags a known, old successful fetch', () => {
    const now = new Date('2026-11-10T00:00:00Z');
    expect(isStale('2026-11-09T23:00:00Z', 120, now)).toBe(false);
    expect(isStale('2026-11-08T00:00:00Z', 120, now)).toBe(true);
    expect(isStale(null, 120, now)).toBe(false);
  });
});

describe('normalizeTimestamp', () => {
  it('treats the Go zero time as unknown', () => {
    expect(normalizeTimestamp('0001-01-01T00:00:00Z')).toBeNull();
    expect(normalizeTimestamp('')).toBeNull();
    expect(normalizeTimestamp(null)).toBeNull();
    expect(normalizeTimestamp('2026-10-21T00:00:00Z')).toBe('2026-10-21T00:00:00Z');
  });
});

describe('formatApiDateTime', () => {
  it('uses the known IANA timezone when present', () => {
    expect(formatApiDateTime('2026-11-10T04:00:00Z', 'en-GB', 'Asia/Shanghai')).toBe(
      '10/11/2026, 12:00 (Asia/Shanghai)'
    );
  });

  it('preserves an explicit RFC3339 offset when no IANA zone is known', () => {
    expect(formatApiDateTime('2026-11-10T12:00:00+08:00', 'en-GB')).toBe(
      '10/11/2026, 12:00 (UTC+08:00)'
    );
    expect(formatApiDateTime('2026-11-10T04:00:00Z', 'en-GB')).toBe(
      '10/11/2026, 04:00 (UTC)'
    );
  });

  it('returns null for missing timestamps', () => {
    expect(formatApiDateTime(null, 'en-GB', 'UTC')).toBeNull();
  });
});
