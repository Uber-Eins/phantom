/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';

import { canEncodeQrCode } from '@/lib/qr-code';

describe('canEncodeQrCode', () => {
  it('accepts a VLESS encryption link that contains the ML-KEM algorithm prefix', () => {
    const link = [
      'vless://11111111-2222-3333-4444-555555555555@example.test:443',
      '?type=tcp',
      `&encryption=mlkem768x25519plus.native.0rtt.${'A'.repeat(43)}`,
      '&security=reality',
      '&pbk=public-key',
      '&fp=chrome',
      '&sni=example.test',
      '&sid=0123456789abcdef',
      '#client',
    ].join('');

    expect(canEncodeQrCode(link)).toBe(true);
  });

  it('rejects only values that exceed the capacity of one QR symbol', () => {
    expect(canEncodeQrCode('a'.repeat(2953))).toBe(true);
    expect(canEncodeQrCode('a'.repeat(2954))).toBe(false);
    expect(canEncodeQrCode('你'.repeat(984))).toBe(true);
    expect(canEncodeQrCode('你'.repeat(985))).toBe(false);
  });
});
