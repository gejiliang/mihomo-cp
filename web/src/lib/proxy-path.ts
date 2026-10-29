/** Path (relative to `/api`) for a runtime delay test of one proxy.
 *
 * The name is percent-encoded exactly once: subscription node names may contain
 * spaces, `/`, `+`, `#` or non-ASCII characters, and mihomo matches the proxy by
 * its real (decoded) name. Encoding here keeps the name in a single path
 * segment without changing what the backend forwards upstream.
 */
export function proxyDelayPath(name: string): string {
  return `runtime/proxies/${encodeURIComponent(name)}/delay`;
}
