import { describe, expect, it } from 'vitest';
import { providerHealthcheckPath, proxyDelayPath } from './proxy-path';

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

describe('providerHealthcheckPath', () => {
  it('keeps provider and node as separate, individually encoded segments', () => {
    const path = providerHealthcheckPath('Demo/Provider +1', 'Demo HK / 01 + #1 节点');
    expect(path).toBe(
      'runtime/proxy-providers/Demo%2FProvider%20%2B1/Demo%20HK%20%2F%2001%20%2B%20%231%20%E8%8A%82%E7%82%B9/healthcheck'
    );
    const segments = path.replace('runtime/proxy-providers/', '').replace('/healthcheck', '').split('/');
    expect(segments).toHaveLength(2);
    expect(decodeURIComponent(segments[0])).toBe('Demo/Provider +1');
    expect(decodeURIComponent(segments[1])).toBe('Demo HK / 01 + #1 节点');
  });

  it('addresses same-named nodes under different providers with different paths', () => {
    const a = providerHealthcheckPath('Demo Provider', 'Shared Node');
    const b = providerHealthcheckPath('Other Provider', 'Shared Node');
    expect(a).not.toBe(b);
    expect(a).toBe('runtime/proxy-providers/Demo%20Provider/Shared%20Node/healthcheck');
    expect(b).toBe('runtime/proxy-providers/Other%20Provider/Shared%20Node/healthcheck');
  });
});
