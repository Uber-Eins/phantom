import { RandomUtil } from '@/utils';
import { GrpcStreamSettingsSchema } from '@/schemas/protocols/stream/grpc';
import { TcpStreamSettingsSchema } from '@/schemas/protocols/stream/tcp';
import { WsStreamSettingsSchema } from '@/schemas/protocols/stream/ws';
import { XHttpStreamSettingsSchema } from '@/schemas/protocols/stream/xhttp';

import type { GuidedDraft, GuidedSecurity, GuidedTemplate } from './types';

export const GUIDED_TEMPLATE_OPTIONS: Array<{ value: GuidedTemplate; label: string }> = [
  { value: 'vless-tcp-tls', label: 'VLESS-TCP-TLS' },
  { value: 'vless-tcp-reality', label: 'VLESS-TCP-REALITY' },
  { value: 'vless-ws-tls', label: 'VLESS-WS-TLS' },
  { value: 'vless-grpc-tls', label: 'VLESS-gRPC-TLS' },
  { value: 'vless-grpc-reality', label: 'VLESS-gRPC-REALITY' },
  { value: 'vless-xhttp-tls', label: 'VLESS-XHTTP-TLS' },
  { value: 'vless-xhttp-reality', label: 'VLESS-XHTTP-REALITY' },
];

export function guidedTemplateName(template: GuidedTemplate): string {
  return GUIDED_TEMPLATE_OPTIONS.find((option) => option.value === template)!.label;
}

export function guidedSocketPath(template: GuidedTemplate): string {
  return `/run/xray/${guidedTemplateName(template)}`;
}

export function guidedTemplateSecurity(template: GuidedTemplate): GuidedSecurity {
  return template.endsWith('-reality') ? 'reality' : 'tls';
}

export function createGuidedDraft(template: GuidedTemplate): GuidedDraft {
  const security = guidedTemplateSecurity(template);
  return {
    template,
    security,
    decoyMode: security === 'reality' ? 'reality-target' : 'unauthorized',
    decoyValue: '',
  };
}

export function createGuidedStreamBase(template: GuidedTemplate): Record<string, unknown> {
  const segment = RandomUtil.randomLowerAndNum(12);
  if (template.includes('-ws-')) {
    return {
      network: 'ws',
      security: 'none',
      wsSettings: WsStreamSettingsSchema.parse({ path: `/${segment}` }),
    };
  }
  if (template.includes('-grpc-')) {
    return {
      network: 'grpc',
      security: 'none',
      grpcSettings: GrpcStreamSettingsSchema.parse({ serviceName: segment }),
    };
  }
  if (template.includes('-xhttp-')) {
    return {
      network: 'xhttp',
      security: 'none',
      xhttpSettings: XHttpStreamSettingsSchema.parse({ path: `/${segment}` }),
    };
  }
  return {
    network: 'tcp',
    security: 'none',
    tcpSettings: TcpStreamSettingsSchema.parse({ header: { type: 'none' } }),
  };
}
