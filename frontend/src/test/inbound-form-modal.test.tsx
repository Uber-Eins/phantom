import { describe, it, expect } from 'vitest';
import { act, waitFor } from '@testing-library/react';

import InboundFormModal from '@/pages/inbounds/form/InboundFormModal';
import {
  renderWithProviders,
  fieldLabels,
  listSelectOptions,
  chooseSelectOption,
} from './test-utils';

function renderModal() {
  return renderWithProviders(
    <InboundFormModal
      open
      mode="add"
      dbInbound={null}
      dbInbounds={[]}
      onClose={() => {}}
      onSaved={() => {}}
    />,
  );
}

describe('InboundFormModal', () => {
  it('renders add mode without crashing', () => {
    renderModal();
    expect(document.querySelector('.ant-modal')).toBeTruthy();
    expect(fieldLabels().length).toBeGreaterThan(0);
  });

  it('field structure differs per protocol (not a vacuous snapshot loop)', async () => {
    renderModal();
    const protocols = listSelectOptions('protocol');
    expect(protocols.length).toBeGreaterThan(3);

    const labelsByProto: Record<string, string[]> = {};
    for (const proto of protocols) {
      chooseSelectOption('protocol', proto);
      // Flush antd Form.useWatch('protocol') before reading — without it every iteration
      // sees the same pre-update DOM and the loop asserts nothing (the original bug here).
      await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
      labelsByProto[proto] = fieldLabels();
    }

    // The loop must actually exercise protocol-specific rendering: distinct protocols
    // must yield distinct field sets (a vacuous loop makes them all identical).
    const distinctShapes = new Set(Object.values(labelsByProto).map((l) => l.join('|')));
    expect(distinctShapes.size).toBeGreaterThan(1);

    // Spot-check a protocol-distinguishing field that must appear after the switch.
    if (labelsByProto.shadowsocks) {
      expect(labelsByProto.shadowsocks).toContain('Encryption method');
    }
  }, 30000); // iterates every protocol, re-rendering a heavy modal each time — slow on CI runners

  it('exposes the reused VLESS Enc fields in the guide', async () => {
    renderModal();
    chooseSelectOption('guided-template', 'VLESS-XHTTP-TLS');

    await waitFor(() => {
      expect(fieldLabels()).toEqual(expect.arrayContaining([
        'Decryption',
        'Encryption',
        'Generate keys',
      ]));
    });
  });

});
