import { describe, expect, it } from 'vitest';
import { proxyDelayPath } from './proxy-path';

describe('proxyDelayPath', () => {
  it('keeps a plain node name as one path segment', () => {
    expect(proxyDelayPath('HK-01')).toBe('runtime/proxies/HK-01/delay');
  });

  it('encodes spaces, slashes, plus signs, hashes and non-ASCII names exactly once', () => {
    const path = proxyDelayPath('Demo HK / 01 + #1 节点');
    expect(path).toBe(
      'runtime/proxies/Demo%20HK%20%2F%2001%20%2B%20%231%20%E8%8A%82%E7%82%B9/delay'
    );
    // The encoded name must stay a single segment: it is the real node name,
    // not an id, so it must round-trip through decodeURIComponent.
    const encoded = path.replace('runtime/proxies/', '').replace('/delay', '');
    expect(encoded).not.toContain('/');
    expect(decodeURIComponent(encoded)).toBe('Demo HK / 01 + #1 节点');
  });
});
