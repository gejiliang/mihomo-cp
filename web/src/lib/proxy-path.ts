/** Path (relative to `/api`) for a runtime delay test of a static proxy.
 *
 * The name is percent-encoded exactly once: node names may contain spaces, `/`,
 * `+`, `#` or non-ASCII characters, and mihomo matches the proxy by its real
 * (decoded) name. Encoding here keeps the name in a single path segment
 * without changing what the backend forwards upstream.
 */
export function proxyDelayPath(name: string): string {
  return `runtime/proxies/${encodeURIComponent(name)}/delay`;
}

/** Path (relative to `/api`) for a subscription (native provider) node delay
 * test.
 *
 * Provider and node are encoded as separate segments: the backend proxies them
 * to mihomo's provider-scoped `GET /providers/proxies/{provider}/{node}/healthcheck`,
 * so the same node name under two providers resolves to two different upstream
 * paths.
 */
export function providerHealthcheckPath(provider: string, node: string): string {
  return `runtime/proxy-providers/${encodeURIComponent(provider)}/${encodeURIComponent(node)}/healthcheck`;
}
