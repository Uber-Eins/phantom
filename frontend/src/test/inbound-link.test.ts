/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';

import {
  genHysteriaLink,
  genInboundLinks,
  genShadowsocksLink,
  genTrojanLink,
  genVlessLink,
  genVmessLink,
  genWireguardConfig,
  genWireguardLink,
  resolveAddr,
} from '@/lib/xray/inbound-link';
import { InboundSchema } from '@/schemas/api/inbound';
import type { WireguardInboundSettings } from '@/schemas/protocols/inbound/wireguard';

// Snapshot baseline for the share-link generators. Snapshots were locked
// at the close of the legacy class migration — at that point each
// generator was verified byte-equal to the corresponding legacy Inbound
// class method. Future drift past this baseline is a regression.

const fullFixtures = import.meta.glob<unknown>(
  './golden/fixtures/inbound-full/*.json',
  { eager: true, import: 'default' },
);

function fixtureName(path: string): string {
  const file = path.split('/').pop() ?? path;
  return file.replace(/\.json$/, '');
}

function fixturesForProtocol(protocol: string): Array<[string, Record<string, unknown>]> {
  return Object.entries(fullFixtures)
    .filter(([, raw]) => (raw as { protocol?: string }).protocol === protocol)
    .map(([path, raw]): [string, Record<string, unknown>] => [fixtureName(path), raw as Record<string, unknown>])
    .sort(([a], [b]) => a.localeCompare(b));
}

describe('genVmessLink', () => {
  const fixtures = fixturesForProtocol('vmess');
  expect(fixtures.length, 'need at least one vmess full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const settings = (raw as { settings: { clients: Array<{ id: string; security?: string }> } }).settings;
      const client = settings.clients[0];

      const link = genVmessLink({
        inbound: typed,
        address: 'example.test',
        port: typed.port,
        forceTls: 'same',
        remark: 'parity-test',
        clientId: client.id,
        security: client.security as never,
      });
      expect(link).toMatchSnapshot();
    });
  }
});

describe('genVlessLink', () => {
  const fixtures = fixturesForProtocol('vless');
  expect(fixtures.length, 'need at least one vless full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const settings = (raw as { settings: { clients: Array<{ id: string; flow?: string }> } }).settings;
      const client = settings.clients[0];

      const link = genVlessLink({
        inbound: typed,
        address: 'example.test',
        port: typed.port,
        forceTls: 'same',
        remark: 'parity-test',
        clientId: client.id,
        flow: client.flow as never,
      });
      expect(link).toMatchSnapshot();
    });
  }
});

describe('genTrojanLink', () => {
  const fixtures = fixturesForProtocol('trojan');
  expect(fixtures.length, 'need at least one trojan full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const settings = (raw as { settings: { clients: Array<{ password: string }> } }).settings;
      const client = settings.clients[0];

      const link = genTrojanLink({
        inbound: typed,
        address: 'example.test',
        port: typed.port,
        forceTls: 'same',
        remark: 'parity-test',
        clientPassword: client.password,
      });
      expect(link).toMatchSnapshot();
    });
  }
});

describe('genHysteriaLink', () => {
  const fixtures = fixturesForProtocol('hysteria');
  expect(fixtures.length, 'need at least one hysteria full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const settings = (raw as { settings: { clients: Array<{ auth: string }> } }).settings;
      const client = settings.clients[0];

      const link = genHysteriaLink({
        inbound: typed,
        address: 'example.test',
        port: typed.port,
        remark: 'parity-test',
        clientAuth: client.auth,
      });
      expect(link).toMatchSnapshot();
    });
  }

  it('emits the UDP hop range as the v2rayN-compatible mport param', () => {
    const [, raw] = fixtures[0];
    const withHop = {
      ...raw,
      settings: { ...(raw.settings as Record<string, unknown>), version: 2 },
      streamSettings: {
        ...(raw.streamSettings as Record<string, unknown>),
        finalmask: { quicParams: { udpHop: { ports: '20000-50000', interval: '5-10' } } },
      },
    };
    const typed = InboundSchema.parse(withHop);
    const client = (raw.settings as { clients: Array<{ auth: string }> }).clients[0];

    const link = genHysteriaLink({
      inbound: typed,
      address: 'example.test',
      port: typed.port,
      remark: 'hop-test',
      clientAuth: client.auth,
    });

    expect(link.startsWith('hysteria2://')).toBe(true);
    expect(link).toContain(`@example.test:${typed.port}`);
    expect(link).toContain('mport=20000-50000');
    expect(link.endsWith('#hop-test')).toBe(true);
  });

  it('normalizes pinSHA256 to hex for base64, raw-hex and colon-hex pins (issue #4818)', () => {
    const [, raw] = fixtures[0];
    const base64Pin = 'yEfdI5XQl4wHgLggHEsomosoFZfUfCdfLXfT+W2N6cQ=';
    const hexPin = '84491c0312d9e70f519ce24659a2ca7d9c4ec59dc86417ece426945e0f939293';
    const colonPin = 'C8:47:DD:23:95:D0:97:8C:07:80:B8:20:1C:4B:28:9A:8B:28:15:97:D4:7C:27:5F:2D:77:D3:F9:6D:8D:E9:C4';
    const stream = raw.streamSettings as Record<string, unknown>;
    const tls = stream.tlsSettings as Record<string, unknown>;
    const tlsClientSettings = tls.settings as Record<string, unknown>;
    const withPins = {
      ...raw,
      streamSettings: {
        ...stream,
        tlsSettings: {
          ...tls,
          settings: { ...tlsClientSettings, pinnedPeerCertSha256: [base64Pin, hexPin, colonPin] },
        },
      },
    };
    const typed = InboundSchema.parse(withPins);
    const client = (raw.settings as { clients: Array<{ auth: string }> }).clients[0];

    const link = genHysteriaLink({
      inbound: typed,
      address: 'example.test',
      port: typed.port,
      remark: 'pin-test',
      clientAuth: client.auth,
    });

    const pin = new URL(link).searchParams.get('pinSHA256');
    expect(pin).toBe(
      'c847dd2395d0978c0780b8201c4b289a8b281597d47c275f2d77d3f96d8de9c4,' +
        '84491c0312d9e70f519ce24659a2ca7d9c4ec59dc86417ece426945e0f939293,' +
        'c847dd2395d0978c0780b8201c4b289a8b281597d47c275f2d77d3f96d8de9c4',
    );
  });

});

describe('genWireguardLink + genWireguardConfig', () => {
  const fixtures = fixturesForProtocol('wireguard');
  expect(fixtures.length, 'need at least one wireguard full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      if (typed.protocol !== 'wireguard') throw new Error('not a wireguard fixture');
      // InboundSchema is an intersection of two DUs, so TS can't auto-narrow
      // `settings` from `protocol`. The runtime guard above is the real
      // check; this cast just helps the type checker.
      const settings = typed.settings as WireguardInboundSettings;

      const link = genWireguardLink({
        settings,
        address: 'wg.example.test',
        port: typed.port,
        remark: 'wg-peer-1',
        peerIndex: 0,
      });
      const config = genWireguardConfig({
        settings,
        address: 'wg.example.test',
        port: typed.port,
        remark: 'wg-peer-1',
        peerIndex: 0,
      });
      expect({ link, config }).toMatchSnapshot();
    });
  }
});

describe('genWireguardLink + genWireguardConfig multi allowedIPs', () => {
  const settings = {
    secretKey: '',
    mtu: 1280,
    dns: '',
    peers: [
      {
        privateKey: 'cLI',
        allowedIPs: ['10.0.0.2/32', 'fd00::2/128'],
      },
    ],
  } as unknown as WireguardInboundSettings;

  it('joins every allowed IP into the share-link address param', () => {
    const link = genWireguardLink({
      settings,
      address: 'wg.example.test',
      port: 51820,
      remark: 'dual-stack',
      peerIndex: 0,
    });
    const u = new URL(link);
    expect(u.searchParams.get('address')).toBe('10.0.0.2/32,fd00::2/128');
  });

  it('joins every allowed IP into the .conf Address line', () => {
    const config = genWireguardConfig({
      settings,
      address: 'wg.example.test',
      port: 51820,
      remark: 'dual-stack',
      peerIndex: 0,
    });
    expect(config).toContain('Address = 10.0.0.2/32, fd00::2/128\n');
  });
});

describe('resolveAddr', () => {
  const baseInbound = {
    listen: '',
    port: 443,
    protocol: 'vless' as const,
  };

  it('uses a shareable local listen address', () => {
    expect(resolveAddr(
      { ...baseInbound, listen: '10.0.0.1' } as never,
      'fallback.test',
    )).toBe('10.0.0.1');
  });

  it('skips listen when it is 0.0.0.0 and falls through to fallbackHostname', () => {
    expect(resolveAddr(
      { ...baseInbound, listen: '0.0.0.0' } as never,
      'fallback.test',
    )).toBe('fallback.test');
  });

  it('skips a unix socket path listen and falls through to fallbackHostname', () => {
    expect(resolveAddr(
      { ...baseInbound, listen: '/run/xray/in.sock' } as never,
      'fallback.test',
    )).toBe('fallback.test');
    expect(resolveAddr(
      { ...baseInbound, listen: '@xray-abstract' } as never,
      'fallback.test',
    )).toBe('fallback.test');
  });

  it('falls through to fallbackHostname when listen is empty', () => {
    expect(resolveAddr(
      baseInbound as never,
      'fallback.test',
    )).toBe('fallback.test');
  });

  it('normalizes a shareable IPv6 listen address', () => {
    expect(resolveAddr(
      { ...baseInbound, listen: '[2001:db8::1]' } as never,
      'fallback.test',
    )).toBe('[2001:db8::1]');
  });
});

describe('genInboundLinks orchestrator', () => {
  // Every full-inbound fixture should produce the same \r\n-joined link
  // block at this baseline.
  const fixtures = Object.entries(fullFixtures)
    .map(([path, raw]): [string, Record<string, unknown>] => [fixtureName(path), raw as Record<string, unknown>])
    .sort(([a], [b]) => a.localeCompare(b));

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const block = genInboundLinks({
        inbound: typed,
        remark: 'parity-test',
        fallbackHostname: 'override.test',
      });
      expect(block).toMatchSnapshot();
    });
  }
});

describe('genShadowsocksLink', () => {
  const fixtures = fixturesForProtocol('shadowsocks');
  expect(fixtures.length, 'need at least one shadowsocks full-inbound fixture').toBeGreaterThan(0);

  for (const [name, raw] of fixtures) {
    it(`${name}: byte-stable`, () => {
      const typed = InboundSchema.parse(raw);
      const settings = (raw as { settings: { clients?: Array<{ password: string }> } }).settings;
      const client = settings.clients?.[0];

      const link = genShadowsocksLink({
        inbound: typed,
        address: 'example.test',
        port: typed.port,
        forceTls: 'same',
        remark: 'parity-test',
        clientPassword: client?.password ?? '',
      });
      expect(link).toMatchSnapshot();
    });
  }
});

describe('IPv6 bracket wrapping in share-link authority', () => {
  it('genVlessLink brackets a bare IPv6 address', () => {
    const [, raw] = fixturesForProtocol('vless')[0];
    const typed = InboundSchema.parse(raw);
    const clientId = (raw as { settings: { clients: Array<{ id: string }> } }).settings.clients[0].id;

    const link = genVlessLink({
      inbound: typed,
      address: '2001:db8::1',
      port: 443,
      clientId,
    });
    expect(new URL(link).host).toBe('[2001:db8::1]:443');
  });

  it('genTrojanLink brackets a bare IPv6 address', () => {
    const [, raw] = fixturesForProtocol('trojan')[0];
    const typed = InboundSchema.parse(raw);
    const clientPassword = (raw as { settings: { clients: Array<{ password: string }> } }).settings.clients[0].password;

    const link = genTrojanLink({
      inbound: typed,
      address: '2001:db8::1',
      port: 443,
      clientPassword,
    });
    expect(new URL(link).host).toBe('[2001:db8::1]:443');
  });

  it('genShadowsocksLink brackets a bare IPv6 address', () => {
    const [, raw] = fixturesForProtocol('shadowsocks')[0];
    const typed = InboundSchema.parse(raw);
    const clientPassword = (raw as { settings: { clients?: Array<{ password: string }> } }).settings.clients?.[0]?.password ?? '';

    const link = genShadowsocksLink({
      inbound: typed,
      address: '2001:db8::1',
      port: 443,
      clientPassword,
    });
    expect(new URL(link).host).toBe('[2001:db8::1]:443');
  });

  it('genHysteriaLink brackets a bare IPv6 address', () => {
    const [, raw] = fixturesForProtocol('hysteria')[0];
    const typed = InboundSchema.parse(raw);
    const clientAuth = (raw as { settings: { clients: Array<{ auth: string }> } }).settings.clients[0].auth;

    const link = genHysteriaLink({
      inbound: typed,
      address: '2001:db8::1',
      port: 443,
      clientAuth,
    });
    expect(new URL(link).host).toBe('[2001:db8::1]:443');
  });

  it('genWireguardLink brackets a bare IPv6 address', () => {
    const [, raw] = fixturesForProtocol('wireguard')[0];
    const typed = InboundSchema.parse(raw);
    if (typed.protocol !== 'wireguard') throw new Error('not a wireguard fixture');
    const settings = typed.settings as WireguardInboundSettings;

    const link = genWireguardLink({
      settings,
      address: '2001:db8::1',
      port: 443,
      peerIndex: 0,
    });
    expect(new URL(link).host).toBe('[2001:db8::1]:443');
  });

  it('does not bracket IPv4 addresses or hostnames', () => {
    const [, raw] = fixturesForProtocol('vless')[0];
    const typed = InboundSchema.parse(raw);
    const clientId = (raw as { settings: { clients: Array<{ id: string }> } }).settings.clients[0].id;

    const v4 = genVlessLink({ inbound: typed, address: '203.0.113.7', port: 443, clientId });
    expect(new URL(v4).host).toBe('203.0.113.7:443');

    const host = genVlessLink({ inbound: typed, address: 'example.test', port: 443, clientId });
    expect(new URL(host).host).toBe('example.test:443');
  });
});

// #5322: the panel copy-link must carry XTLS Vision `flow` for VLESS+XHTTP
// when VLESS encryption (vlessenc) is on, matching the form's flow display
// and the backend connection-link generator. Gating is via canEnableTlsFlow.
describe('genVlessLink flow gating (#5322)', () => {
  function vlessXhttp(encryption: string) {
    return InboundSchema.parse({
      id: 1,
      up: 0,
      down: 0,
      total: 0,
      remark: 'vlessenc',
      enable: true,
      expiryTime: 0,
      listen: '',
      port: 443,
      tag: 'inbound-vless-xhttp',
      sniffing: {
        enabled: false,
        destOverride: [],
        metadataOnly: false,
        routeOnly: false,
        ipsExcluded: [],
        domainsExcluded: [],
      },
      protocol: 'vless',
      settings: {
        clients: [
          {
            id: '11111111-2222-3333-4444-555555555555',
            email: 'a@example.test',
            flow: 'xtls-rprx-vision',
            limitIp: 0,
            totalGB: 0,
            expiryTime: 0,
            enable: true,
            tgId: 0,
            subId: 's1',
            comment: '',
            reset: 0,
          },
        ],
        decryption: 'none',
        encryption,
        fallbacks: [],
      },
      streamSettings: {
        network: 'xhttp',
        xhttpSettings: {},
        security: 'none',
      },
    });
  }

  const clientId = '11111111-2222-3333-4444-555555555555';

  it('emits flow for VLESS+XHTTP when vless encryption is enabled', () => {
    const link = genVlessLink({
      inbound: vlessXhttp('mlkem768x25519plus.native.0rtt.SGVsbG8'),
      address: 'example.test',
      port: 443,
      clientId,
      flow: 'xtls-rprx-vision',
    });
    expect(new URL(link).searchParams.get('flow')).toBe('xtls-rprx-vision');
  });

  it('omits flow for VLESS+XHTTP without vless encryption', () => {
    const link = genVlessLink({
      inbound: vlessXhttp('none'),
      address: 'example.test',
      port: 443,
      clientId,
      flow: 'xtls-rprx-vision',
    });
    expect(new URL(link).searchParams.has('flow')).toBe(false);
  });

  it('still emits flow for classic TCP+REALITY Vision', () => {
    const [, raw] = fixturesForProtocol('vless').find(([name]) => name === 'vless-tcp-reality')!;
    const typed = InboundSchema.parse(raw);
    const link = genVlessLink({
      inbound: typed,
      address: 'example.test',
      port: 443,
      clientId: (raw as { settings: { clients: Array<{ id: string }> } }).settings.clients[0].id,
      flow: 'xtls-rprx-vision',
    });
    expect(new URL(link).searchParams.get('flow')).toBe('xtls-rprx-vision');
  });
});

describe('genVlessLink XHTTP extra compatibility', () => {
  it('emits both sessionID and legacy session keys in XHTTP extra', () => {
    const typed = InboundSchema.parse({
      id: 1,
      up: 0,
      down: 0,
      total: 0,
      remark: 'xhttp-session',
      enable: true,
      expiryTime: 0,
      listen: '',
      port: 443,
      tag: 'inbound-vless-xhttp',
      sniffing: {
        enabled: false,
        destOverride: [],
        metadataOnly: false,
        routeOnly: false,
        ipsExcluded: [],
        domainsExcluded: [],
      },
      protocol: 'vless',
      settings: {
        clients: [
          {
            id: '11111111-2222-3333-4444-555555555555',
            email: 'a@example.test',
            flow: '',
            limitIp: 0,
            totalGB: 0,
            expiryTime: 0,
            enable: true,
            tgId: 0,
            subId: 's1',
            comment: '',
            reset: 0,
          },
        ],
        decryption: 'none',
        encryption: 'none',
        fallbacks: [],
      },
      streamSettings: {
        network: 'xhttp',
        security: 'none',
        xhttpSettings: {
          path: '/sp',
          host: 'edge.example.test',
          mode: 'auto',
          sessionIDPlacement: 'header',
          sessionIDKey: 'X-Session',
        },
      },
    });

    const link = genVlessLink({
      inbound: typed,
      address: 'example.test',
      port: 443,
      clientId: '11111111-2222-3333-4444-555555555555',
    });
    const extra = JSON.parse(new URL(link).searchParams.get('extra') ?? '{}') as Record<string, unknown>;

    expect(extra.sessionIDPlacement).toBe('header');
    expect(extra.sessionIDKey).toBe('X-Session');
    expect(extra.sessionPlacement).toBe('header');
    expect(extra.sessionKey).toBe('X-Session');
  });
});
