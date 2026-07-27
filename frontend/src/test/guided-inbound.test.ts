import { describe, expect, it } from 'vitest';

import {
  buildGuidedInboundRequest,
  createGuidedDraft,
  createGuidedStreamBase,
  guidedSocketPath,
  guidedTemplateName,
} from '@/pages/inbounds/form/guided';

describe('guided VLESS templates', () => {
  it.each([
    ['vless-tcp-tls', 'tcp', 'tls'],
    ['vless-tcp-reality', 'tcp', 'reality'],
    ['vless-ws-tls', 'ws', 'tls'],
    ['vless-grpc-tls', 'grpc', 'tls'],
    ['vless-grpc-reality', 'grpc', 'reality'],
    ['vless-xhttp-tls', 'xhttp', 'tls'],
    ['vless-xhttp-reality', 'xhttp', 'reality'],
  ] as const)('%s seeds %s + %s', (template, network, security) => {
    const draft = createGuidedDraft(template);
    const stream = createGuidedStreamBase(template);

    expect(draft.security).toBe(security);
    expect(draft.decoyMode).toBe(security === 'tls' ? 'unauthorized' : 'reality-target');
    expect(stream.network).toBe(network);
    expect(stream.security).toBe('none');
    expect(stream[`${network}Settings`]).toBeTypeOf('object');
  });

  it('builds the nested API payload as JSON', () => {
    const draft = createGuidedDraft('vless-xhttp-reality');
    const request = buildGuidedInboundRequest({
      up: 0,
      down: 0,
      total: 0,
      remark: '',
      enable: true,
      expiryTime: 0,
      trafficReset: 'never',
      lastTrafficResetTime: 0,
      listen: guidedSocketPath(draft.template),
      port: 0,
      protocol: 'vless',
      settings: '{}',
      streamSettings: '{}',
      sniffing: '{}',
      tag: guidedTemplateName(draft.template),
    }, draft);

    expect(request.options).toEqual({
      headers: { 'Content-Type': 'application/json' },
    });
    expect(request.body.fronting).toEqual({
      template: 'vless-xhttp-reality',
      decoyMode: 'reality-target',
      decoyValue: '',
    });
    expect(request.body.inbound).toMatchObject({
      listen: '/run/xray/VLESS-XHTTP-REALITY',
      port: 0,
      tag: 'VLESS-XHTTP-REALITY',
    });
  });
});
