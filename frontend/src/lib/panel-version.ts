// Format a panel version for display. Dev builds report a "dev+<commit>"
// identity (see config.GetPanelVersion); show those — and any other
// non-numeric label — verbatim. Semantic versions get a single normalized "v"
// prefix, so a raw "v3.4.0" tag and a bare "3.4.0" both render as "v3.4.0"
// instead of doubling up to "vv3.4.0".
export function formatPanelVersion(version: string | undefined | null): string {
  const v = (version || '').trim();
  if (!v) return '';
  const normalized = v.replace(/^v/i, '');
  return /^\d/.test(normalized) ? `v${normalized}` : v;
}
