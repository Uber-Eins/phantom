import type { WireInboundPayload } from '@/lib/xray/inbound-form-adapter';

import type { GuidedDraft } from './types';

export function buildGuidedInboundRequest(
  inbound: WireInboundPayload,
  draft: GuidedDraft,
) {
  return {
    body: {
      inbound,
      fronting: {
        template: draft.template,
        decoyMode: draft.decoyMode,
        decoyValue: draft.decoyValue.trim(),
      },
    },
    options: {
      headers: { 'Content-Type': 'application/json' },
    },
  };
}
