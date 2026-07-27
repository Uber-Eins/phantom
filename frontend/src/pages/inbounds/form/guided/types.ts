export const GUIDED_TEMPLATES = [
  'vless-tcp-tls',
  'vless-tcp-reality',
  'vless-ws-tls',
  'vless-grpc-tls',
  'vless-grpc-reality',
  'vless-xhttp-tls',
  'vless-xhttp-reality',
] as const;

export type GuidedTemplate = (typeof GUIDED_TEMPLATES)[number];
export type GuidedSecurity = 'tls' | 'reality';
export type GuidedDecoyMode = 'unauthorized' | 'proxy' | 'static' | 'reality-target';

export interface GuidedDraft {
  template: GuidedTemplate;
  security: GuidedSecurity;
  decoyMode: GuidedDecoyMode;
  decoyValue: string;
}

export interface GuidedFrontingPayload {
  template: GuidedTemplate;
  decoyMode: GuidedDecoyMode;
  decoyValue: string;
}
