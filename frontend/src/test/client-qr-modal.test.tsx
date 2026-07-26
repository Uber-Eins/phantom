import { waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import ClientQrModal from '@/pages/clients/ClientQrModal';
import { HttpUtil } from '@/utils';
import { renderWithProviders } from './test-utils';

describe('ClientQrModal', () => {
  it('renders a QR code for an encodable VLESS encryption link', async () => {
    const link = [
      'vless://11111111-2222-3333-4444-555555555555@example.test:443',
      '?type=tcp',
      `&encryption=mlkem768x25519plus.native.0rtt.${'A'.repeat(43)}`,
      '&security=reality',
      '&pbk=public-key',
      '&fp=chrome',
      '&sni=example.test',
      '&sid=0123456789abcdef',
      '#jej4n0adek',
    ].join('');
    vi.mocked(HttpUtil.get).mockResolvedValueOnce({ success: true, msg: '', obj: [link] });

    renderWithProviders(
      <ClientQrModal
        open
        client={{ email: 'jej4n0adek', inboundIds: [1] }}
        inboundsById={{
          1: {
            id: 1,
            protocol: 'vless',
            port: 443,
            remark: 'DIRECT',
          },
        }}
        onOpenChange={() => {}}
      />,
    );

    await waitFor(() => {
      expect(document.querySelector('.qr-panel-canvas svg')).not.toBeNull();
    });
  });
});
