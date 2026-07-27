import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { QuestionCircleOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import {
  Alert,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Switch,
  Tabs,
  Tooltip,
  message,
} from 'antd';
import { Controller, FormProvider, useForm, useWatch } from 'react-hook-form';

import { HttpUtil, NumberFormatter, RandomUtil, SizeFormatter, Wireguard } from '@/utils';
import type { RealityScanResult } from '@/models/reality-scan';
import {
  rawInboundToFormValues,
  formValuesToWirePayload,
} from '@/lib/xray/inbound-form-adapter';
import { createDefaultInboundSettings } from '@/lib/xray/inbound-defaults';
import { composeInboundTag, isAutoInboundTag, type InboundTagInput } from '@/lib/xray/inbound-tag';
import {
  canEnableReality,
  canEnableSniffing,
  canEnableStream,
  canEnableTls,
  isSS2022,
} from '@/lib/xray/protocol-capabilities';
import {
  InboundFormBaseSchema,
  InboundFormSchema,
  type InboundFormValues,
} from '@/schemas/forms/inbound-form';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { Protocols } from '@/schemas/primitives';
import { SockoptStreamSettingsSchema } from '@/schemas/protocols/stream/sockopt';
import { HysteriaStreamSettingsSchema } from '@/schemas/protocols/stream/hysteria';
import { createHysteriaTlsSettingsWithDefaultCert } from '@/lib/xray/inbound-tls-defaults';
import { VLESS_AUTH_LABEL_KEYS, vlessEncryptionAuthKind } from '@/lib/xray/vless-encryption';
import { SniffingSchema } from '@/schemas/primitives/sniffing';
import { TcpStreamSettingsSchema } from '@/schemas/protocols/stream/tcp';
import { KcpStreamSettingsSchema } from '@/schemas/protocols/stream/kcp';
import { WsStreamSettingsSchema } from '@/schemas/protocols/stream/ws';
import { GrpcStreamSettingsSchema } from '@/schemas/protocols/stream/grpc';
import { HttpUpgradeStreamSettingsSchema } from '@/schemas/protocols/stream/httpupgrade';
import { XHttpStreamSettingsSchema } from '@/schemas/protocols/stream/xhttp';
import { DateTimePicker } from '@/components/form';
import { FinalMaskField } from '@/lib/xray/forms/fields';
import './InboundFormModal.css';

import { AdvancedAllEditor, AdvancedSliceEditor } from './advanced-editors';
import { formatInboundIssue, formatInboundValidation } from './formatValidationError';
import {
  HttpFields,
  HysteriaFields,
  MixedFields,
  MtprotoFields,
  ShadowsocksFields,
  TunFields,
  TunnelFields,
  VlessFields,
  WireguardFields,
} from './protocols';
import {
  GrpcForm,
  HttpUpgradeForm,
  KcpForm,
  RawForm,
  SockoptForm,
  WsForm,
  XhttpForm,
} from './transport';
import { PrimaryCertificateFields, RealityForm, TlsForm } from './security';
import { useSecurityActions } from './useSecurityActions';
import { useInboundFallbacks } from './useInboundFallbacks';
import FallbacksCard from './FallbacksCard';
import SniffingTab from './SniffingTab';
import {
  createGuidedDraft,
  createGuidedStreamBase,
  buildGuidedInboundRequest,
  guidedSocketPath,
  guidedTemplateName,
  GuidedInboundTab,
  type GuidedDraft,
  type GuidedTemplate,
} from './guided';

import type { DBInbound } from '@/models/dbinbound';


/* Render a field label with a hover tooltip icon instead of an `extra` help line below. */
const labelWithHint = (label: string, hint: string) => (
  <span>
    {label}
    <Tooltip title={hint}>
      <QuestionCircleOutlined style={{ marginInlineStart: 4, color: 'rgba(128,128,128,0.65)' }} />
    </Tooltip>
  </span>
);

const PROTOCOL_OPTIONS = Object.values(Protocols).map((p) => ({ value: p, label: p }));
const TRAFFIC_RESETS = ['never', 'hourly', 'daily', 'weekly', 'monthly'] as const;

interface InboundFormModalProps {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
  mode: 'add' | 'edit';
  dbInbound: DBInbound | null;
  dbInbounds: DBInbound[];
}

function buildAddModeValues(): InboundFormValues {
  const settings = createDefaultInboundSettings('vless') ?? undefined;
  return rawInboundToFormValues({
    protocol: 'vless',
    settings,
    streamSettings: {
      network: 'tcp',
      security: 'none',
      tcpSettings: TcpStreamSettingsSchema.parse({ header: { type: 'none' } }),
    },
    sniffing: SniffingSchema.parse({}),
    port: RandomUtil.randomInteger(10000, 60000),
    listen: '',
    tag: '',
    enable: true,
    trafficReset: 'never',
  });
}

/*
 * Switching `network` swaps which per-network key (tcpSettings, wsSettings,
 * grpcSettings, ...) appears on the wire. Seed each network's blob with its
 * Zod schema defaults so every field inside the network sub-form has a
 * defined starting value (KCP needs MTU=1350 etc., XHTTP needs the ""
 * sentinels so the "Default" option shows instead of blank).
 */
function newStreamSlice(n: string): Record<string, unknown> {
  switch (n) {
    case 'tcp': return TcpStreamSettingsSchema.parse({ header: { type: 'none' } });
    case 'kcp': return KcpStreamSettingsSchema.parse({});
    case 'ws': return WsStreamSettingsSchema.parse({});
    case 'grpc': return GrpcStreamSettingsSchema.parse({});
    case 'httpupgrade': return HttpUpgradeStreamSettingsSchema.parse({});
    case 'xhttp': return XHttpStreamSettingsSchema.parse({});
    default: return {};
  }
}

export default function InboundFormModal({
  open,
  onClose,
  onSaved,
  mode,
  dbInbound,
  dbInbounds,
}: InboundFormModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const methods = useForm<InboundFormValues>({ defaultValues: buildAddModeValues() });
  const setV = methods.setValue as unknown as (name: string, value: unknown) => void;
  const getV = methods.getValues as unknown as (name?: string) => unknown;
  const control = methods.control;
  const [saving, setSaving] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [scanResult, setScanResult] = useState<RealityScanResult | null>(null);
  const [guidedDraft, setGuidedDraft] = useState<GuidedDraft | null>(null);
  const [activeTab, setActiveTab] = useState(mode === 'add' ? 'guided' : 'basic');
  const {
    fallbacks,
    fallbackChildOptions,
    loadFallbacks,
    saveFallbacks,
    addFallback,
    updateFallback,
    removeFallback,
    moveFallback,
    addAllFallbacks,
  } = useInboundFallbacks(dbInbound, dbInbounds);

  const protocol = (useWatch({ control, name: 'protocol' }) ?? '') as string;
  const vlessEncryption = useWatch({ control, name: 'settings.encryption' }) ?? '';
  const ssMethod = useWatch({ control, name: 'settings.method' });
  const isSSWith2022 = isSS2022({
    protocol,
    settings: typeof ssMethod === 'string' ? { method: ssMethod } : {},
  });
  const mixedUdpOn = (useWatch({ control, name: 'settings.udp' }) ?? false) as boolean;
  const network = (useWatch({ control, name: 'streamSettings.network' }) ?? '') as string;
  const security = (useWatch({ control, name: 'streamSettings.security' }) ?? 'none') as string;
  const streamEnabled = canEnableStream({ protocol });
  const sniffingSupported = canEnableSniffing({ protocol });
  /*
   * Wireguard (always a UDP listener) and Tunnel (dokodemo-door) expose no
   * user-selectable transport — their stream tab is just sockopt, which is all
   * Tunnel's TProxy/redirect mode needs (sockopt.tproxy). Hysteria carries its
   * own dedicated transport form. For all of these the RAW/mKCP/WS/... network
   * picker and the per-network sub-forms are hidden.
   */
  const hasSelectableTransport =
    protocol !== Protocols.HYSTERIA
    && protocol !== Protocols.WIREGUARD
    && protocol !== Protocols.TUNNEL;

  const wPort = useWatch({ control, name: 'port' });
  const wListen = (useWatch({ control, name: 'listen' }) ?? '') as string;
  const isUdsListen = wListen.startsWith('/') || wListen.startsWith('@');
  const wTag = (useWatch({ control, name: 'tag' }) ?? '') as string;
  const wSsNetwork = useWatch({ control, name: 'settings.network' });
  const wTunnelNetwork = useWatch({ control, name: 'settings.allowedNetwork' });
  const wTotal = (useWatch({ control, name: 'total' }) as number | undefined) ?? 0;
  const wExpiry = (useWatch({ control, name: 'expiryTime' }) as number | undefined) ?? 0;
  const autoTagRef = useRef(true);
  const lastWrittenTagRef = useRef('');
  const currentTagInput = (): InboundTagInput => ({
    port: typeof wPort === 'number' ? wPort : 0,
    protocol,
    streamSettings: { network },
    settings: { network: wSsNetwork, allowedNetwork: wTunnelNetwork, udp: mixedUdpOn },
  });
  const isFallbackHost =
    (protocol === Protocols.VLESS || protocol === Protocols.TROJAN)
    && network === 'tcp'
    && (security === 'tls' || security === 'reality');
  const topologyLocked = guidedDraft !== null || (mode === 'edit' && !!dbInbound?.fronting);

  const {
    genRealityKeypair,
    clearRealityKeypair,
    genMldsa65,
    clearMldsa65,
    scanRealityTarget,
    scanRealityCandidates,
    applyRealityScanResult,
    randomizeShortIds,
    randomizeSpiderX,
    getNewEchCert,
    clearEchCert,
    pinFromCert,
    pinFromRemote,
    setCertFromPanel,
    clearCertFiles,
    onSecurityChange,
  } = useSecurityActions({ methods, setSaving, messageApi, setScanResult, setScanning });


  const toggleSockopt = (on: boolean) => {
    if (on) {
      setV('streamSettings.sockopt', SockoptStreamSettingsSchema.parse({}));
    } else {
      setV('streamSettings.sockopt', undefined);
    }
  };
  const wgSecretKey = useWatch({ control, name: 'settings.secretKey' });
  const wgPubKey = typeof wgSecretKey === 'string' && wgSecretKey.length > 0
    ? Wireguard.generateKeypair(wgSecretKey).publicKey
    : '';

  const regenInboundWg = () => {
    const kp = Wireguard.generateKeypair();
    setV('settings.secretKey', kp.privateKey);
  };

  const matchesVlessAuth = (
    block: { id?: string; label?: string } | undefined | null,
    authId: string,
  ) => {
    if (block?.id === authId) return true;
    const label = (block?.label || '').toLowerCase().replace(/[-_\s]/g, '');
    if (authId === 'mlkem768') return label.includes('mlkem768') && !label.includes('xorpub') && !label.includes('random');
    if (authId === 'x25519') return label.includes('x25519') && !label.includes('xorpub') && !label.includes('random');
    if (authId === 'mlkem768_xorpub') return label.includes('mlkem768') && label.includes('xorpub');
    if (authId === 'mlkem768_random') return label.includes('mlkem768') && label.includes('random');
    if (authId === 'x25519_xorpub') return label.includes('x25519') && label.includes('xorpub');
    if (authId === 'x25519_random') return label.includes('x25519') && label.includes('random');
    return false;
  };

  const getNewVlessEnc = async (authId: string) => {
    if (!authId) return;
    setSaving(true);
    try {
      const msg = await HttpUtil.get('/panel/api/server/getNewVlessEnc');
      if (!msg?.success) return;
      const obj = msg.obj as {
        auths?: { decryption: string; encryption: string; label?: string; id?: string }[];
      };
      const block = (obj.auths || []).find((a) => matchesVlessAuth(a, authId));
      if (!block) return;
      setV('settings.decryption', block.decryption);
      setV('settings.encryption', block.encryption);
    } finally {
      setSaving(false);
    }
  };

  const clearVlessEnc = () => {
    setV('settings.decryption', 'none');
    setV('settings.encryption', 'none');
  };

  const vlessAuthKind = vlessEncryptionAuthKind(
    typeof vlessEncryption === 'string' ? vlessEncryption : '',
  );
  const selectedVlessAuth = (() => {
    const enc = typeof vlessEncryption === 'string' ? vlessEncryption : '';
    if (!enc || enc === 'none') return 'None';
    if (!vlessAuthKind) return t('pages.inbounds.vlessAuthCustom');
    return t(VLESS_AUTH_LABEL_KEYS[vlessAuthKind]);
  })();

  useEffect(() => {
    if (!open) return;
    const initial = mode === 'edit' && dbInbound
      ? rawInboundToFormValues(dbInbound)
      : buildAddModeValues();
    methods.reset(initial);
    setGuidedDraft(null);
    setActiveTab(mode === 'add' ? 'guided' : 'basic');
    setScanResult(null);
    const initialTag = (initial.tag ?? '') as string;
    autoTagRef.current = isAutoInboundTag(initialTag, {
      port: initial.port ?? 0,
      protocol: initial.protocol,
      streamSettings: (initial.streamSettings ?? {}) as Record<string, unknown>,
      settings: (initial.settings ?? {}) as Record<string, unknown>,
    });
    lastWrittenTagRef.current = initialTag;
    if (
      mode === 'edit'
      && dbInbound
      && !dbInbound.fronting
      && (dbInbound.protocol === Protocols.VLESS || dbInbound.protocol === Protocols.TROJAN)
    ) {
      loadFallbacks(dbInbound.id);
    } else {
      loadFallbacks(null);
    }

    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [open, mode, dbInbound, methods]);

  useEffect(() => {
    if (!open) return;
    if (wTag === lastWrittenTagRef.current) return;
    autoTagRef.current = isAutoInboundTag(wTag, currentTagInput());
    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [open, wTag]);

  useEffect(() => {
    if (!open || !autoTagRef.current) return;
    const next = composeInboundTag(currentTagInput());
    if (next !== ((getV('tag') as string | undefined) ?? '')) {
      lastWrittenTagRef.current = next;
      setV('tag', next);
    }
    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [open, wPort, protocol, network, mixedUdpOn, wSsNetwork, wTunnelNetwork]);

  /*
   * Protocol picker reset cascades through the form — clearing the settings DU
   * branch. Only a real user change (type === 'change') triggers it;
   * programmatic setValue (advanced
   * JSON edits, open reset) must not, matching the legacy onValuesChange.
   */
  useEffect(() => {
    if (mode === 'edit') return;
    /* eslint-disable-next-line react-hooks/incompatible-library */
    const sub = methods.watch((_value, { name, type }) => {
      if (name !== 'protocol' || type !== 'change') return;
      const next = getV('protocol') as string;
      const settings = createDefaultInboundSettings(next) ?? undefined;
      setV('settings', settings);
      if (next === Protocols.HYSTERIA) {
        setV('streamSettings', {
          network: 'hysteria',
          security: 'tls',
          hysteriaSettings: HysteriaStreamSettingsSchema.parse({}),
          tlsSettings: createHysteriaTlsSettingsWithDefaultCert(),
          finalmask: {
            tcp: [],
            udp: [{
              type: 'salamander',
              settings: { password: RandomUtil.randomLowerAndNum(16) },
            }],
          },
        });
      } else if (next === Protocols.WIREGUARD || next === Protocols.TUNNEL) {
        setV('streamSettings', { security: 'none' });
      } else {
        const current = getV('streamSettings') as { network?: string } | undefined;
        if (current?.network === 'hysteria' || !current?.network) {
          setV('streamSettings', { network: 'tcp', security: 'none', tcpSettings: {} });
        }
      }
    });
    return () => sub.unsubscribe();
    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [mode, methods]);

  const onGuidedTemplateChange = async (template?: GuidedTemplate) => {
    if (!template) {
      autoTagRef.current = true;
      setGuidedDraft(null);
      methods.reset(buildAddModeValues());
      return;
    }

    const draft = createGuidedDraft(template);
    setGuidedDraft(draft);
    setSaving(true);
    try {
      const templateName = guidedTemplateName(template);
      autoTagRef.current = false;
      setV('protocol', Protocols.VLESS);
      setV('port', 0);
      setV('listen', guidedSocketPath(template));
      setV('tag', templateName);
      setV('settings', createDefaultInboundSettings(Protocols.VLESS));
      setV('streamSettings', createGuidedStreamBase(template));
      await onSecurityChange(draft.security);
    } finally {
      setSaving(false);
    }
  };

  const submit = async () => {
    if (mode === 'add' && activeTab === 'guided' && !guidedDraft) {
      messageApi.warning(t('pages.inbounds.guide.templatePlaceholder'));
      return;
    }
    if (!(await methods.trigger())) return;
    /*
     * getValues() returns the entire form store, including settings.clients and
     * settings.fallbacks which have no bound field (clients are managed via the
     * standalone Client modal, not this inbound modal). With shouldUnregister
     * false those pass-through sub-trees survive from the reset object, so the
     * update wire payload never silently drops every client on save.
     */
    const values = methods.getValues() as InboundFormValues;
    const parsed = InboundFormSchema.safeParse(values);
    if (!parsed.success) {
      const issues = parsed.error.issues;
      messageApi.error(formatInboundValidation(issues, values, t));
      console.error(
        '[InboundFormModal] schema validation failed:',
        issues.map((issue) => formatInboundIssue(issue, values, t)),
      );
      return;
    }
    if (
      guidedDraft
      && (guidedDraft.decoyMode === 'proxy' || guidedDraft.decoyMode === 'static')
      && guidedDraft.decoyValue.trim() === ''
    ) {
      messageApi.warning(t('pages.inbounds.guide.decoyValueRequired'));
      return;
    }
    setSaving(true);
    try {
      const payload = formValuesToWirePayload(parsed.data);
      const url = mode === 'edit' && dbInbound
        ? `/panel/api/inbounds/update/${dbInbound.id}`
        : guidedDraft
          ? '/panel/api/inbounds/add-guided'
          : '/panel/api/inbounds/add';
      const guidedRequest = guidedDraft
        ? buildGuidedInboundRequest(payload, guidedDraft)
        : null;
      const msg = await HttpUtil.post(
        url,
        guidedRequest?.body ?? payload,
        guidedRequest?.options ?? {},
      );
      if (msg?.success) {
        if (isFallbackHost && !topologyLocked) {
          const obj = msg.obj as { id?: number; Id?: number } | null;
          const masterId = mode === 'edit'
            ? dbInbound!.id
            : (obj?.id ?? obj?.Id ?? 0);
          if (masterId) await saveFallbacks(masterId);
        }
        onSaved();
        onClose();
      }
    } finally {
      setSaving(false);
    }
  };

  const title = mode === 'edit'
    ? t('pages.inbounds.modifyInbound')
    : t('pages.inbounds.addInbound');

  const okText = mode === 'edit'
    ? t('pages.clients.submitEdit')
    : t('create');

  const basicTab = (
    <>
      <FormField name="enable" label={t('enable')} valueProp="checked">
        <Switch />
      </FormField>

      <FormField name="remark" label={t('pages.inbounds.remark')}>
        <Input />
      </FormField>

      <FormField name="protocol" label={t('pages.inbounds.protocol')}>
        <Select id="protocol" disabled={mode === 'edit' || topologyLocked} options={PROTOCOL_OPTIONS} />
      </FormField>

      <FormField
        name="listen"
        label={labelWithHint(t('pages.inbounds.address'), t('pages.inbounds.form.listenHelp'))}
      >
        <Input disabled={topologyLocked} placeholder={t('pages.inbounds.monitorDesc')} />
      </FormField>

      <FormField
        name="port"
        label={t('pages.inbounds.port')}
        rules={{ validate: rhfZodValidate(InboundFormBaseSchema.shape.port) }}
      >
        <InputNumber disabled={topologyLocked} min={isUdsListen ? 0 : 1} max={65535} />
      </FormField>

      <Form.Item
        label={
          <Tooltip title={t('pages.inbounds.meansNoLimit')}>
            {t('pages.inbounds.totalFlow')}
          </Tooltip>
        }
      >
        <InputNumber
          value={wTotal ? Math.round((wTotal / SizeFormatter.ONE_GB) * 100) / 100 : 0}
          min={0}
          step={1}
          onChange={(v) => {
            const bytes = NumberFormatter.toFixed((Number(v) || 0) * SizeFormatter.ONE_GB, 0);
            setV('total', bytes);
          }}
        />
      </Form.Item>

      <FormField name="trafficReset" label={t('pages.inbounds.periodicTrafficResetTitle')}>
        <Select
          options={TRAFFIC_RESETS.map((r) => ({
            value: r,
            label: t(`pages.inbounds.periodicTrafficReset.${r}`),
          }))}
        />
      </FormField>

      <Form.Item
        label={
          <Tooltip title={t('pages.inbounds.leaveBlankToNeverExpire')}>
            {t('pages.inbounds.expireDate')}
          </Tooltip>
        }
      >
        <DateTimePicker
          value={wExpiry > 0 ? dayjs(wExpiry) : null}
          onChange={(d) => setV('expiryTime', d ? d.valueOf() : 0)}
        />
      </Form.Item>
    </>
  );

  const fallbacksCard = (
    <FallbacksCard
      fallbacks={fallbacks}
      fallbackChildOptions={fallbackChildOptions}
      addFallback={addFallback}
      updateFallback={updateFallback}
      removeFallback={removeFallback}
      moveFallback={moveFallback}
      addAllFallbacks={addAllFallbacks}
    />
  );

  const protocolTab = (
    <>
      {protocol === Protocols.WIREGUARD && <WireguardFields wgPubKey={wgPubKey} regenInboundWg={regenInboundWg} />}

      {protocol === Protocols.TUN && <TunFields />}

      {protocol === Protocols.TUNNEL && <TunnelFields />}

      {protocol === Protocols.HTTP && <HttpFields />}
      {protocol === Protocols.MIXED && <MixedFields mixedUdpOn={mixedUdpOn} />}

      {protocol === Protocols.MTPROTO && <MtprotoFields />}

      {protocol === Protocols.SHADOWSOCKS && <ShadowsocksFields isSSWith2022={isSSWith2022} />}

      {protocol === Protocols.VLESS && <VlessFields saving={saving} selectedVlessAuth={selectedVlessAuth} vlessAuthKind={vlessAuthKind} network={network} security={security} getNewVlessEnc={getNewVlessEnc} clearVlessEnc={clearVlessEnc} />}

      {isFallbackHost && !topologyLocked && fallbacksCard}
      {(protocol === Protocols.VLESS || protocol === Protocols.TROJAN)
        && network === 'tcp' && !isFallbackHost && (
          <Alert
            className="mt-12"
            type="info"
            showIcon
            title={t('pages.inbounds.fallbacks.needsTls')}
          />
        )}
    </>
  );

  /*
   * Switching `network` swaps which per-network key appears on the wire. Clear
   * the old network's blob and seed the new one with schema defaults, plus the
   * FinalMask mkcp-legacy UDP mask when moving to mKCP (removed otherwise).
   */
  const onNetworkChange = (next: string) => {
    const ALL = ['tcpSettings', 'kcpSettings', 'wsSettings', 'grpcSettings', 'httpupgradeSettings', 'xhttpSettings'];
    const current = (getV('streamSettings') as Record<string, unknown>) ?? {};
    const cleaned: Record<string, unknown> = { ...current, network: next };
    for (const k of ALL) {
      if (k !== `${next}Settings`) delete cleaned[k];
    }
    cleaned[`${next}Settings`] = newStreamSlice(next);
    if (next === 'kcp') {
      const fm = (cleaned.finalmask as Record<string, unknown> | undefined) ?? {};
      const udp = Array.isArray(fm.udp) ? (fm.udp as unknown[]) : [];
      const hasMkcp = udp.some((m) => {
        const entry = m as { type?: string };
        return entry?.type === 'mkcp-legacy';
      });
      if (!hasMkcp) {
        cleaned.finalmask = {
          ...fm,
          udp: [...udp, { type: 'mkcp-legacy', settings: { header: '', value: '' } }],
        };
      }
    } else {
      const fm = cleaned.finalmask as Record<string, unknown> | undefined;
      if (fm && Array.isArray(fm.udp)) {
        const udp = (fm.udp as unknown[]).filter((m) => (m as { type?: string })?.type !== 'mkcp-legacy');
        cleaned.finalmask = { ...fm, udp };
      }
    }
    setV('streamSettings', cleaned);
  };

  const streamTab = (
    <>
      {hasSelectableTransport && (
        <Form.Item label={t('transmission')}>
          <Select
            style={{ width: '75%' }}
            value={network}
            disabled={topologyLocked}
            onChange={onNetworkChange}
            options={[
              { value: 'tcp', label: 'RAW' },
              { value: 'kcp', label: 'mKCP' },
              { value: 'ws', label: 'WebSocket' },
              { value: 'grpc', label: 'gRPC' },
              { value: 'httpupgrade', label: 'HTTPUpgrade' },
              { value: 'xhttp', label: 'XHTTP' },
            ]}
          />
        </Form.Item>
      )}

      {/* Inbound Hysteria stream sub-form. The transport for hysteria
          isn't user-selectable (always 'hysteria'), so the network
          dropdown is hidden above. */}
      {protocol === Protocols.HYSTERIA && <HysteriaFields />}

      {hasSelectableTransport && (
        <>
          {network === 'tcp' && <RawForm />}

          {network === 'ws' && <WsForm />}

          {network === 'grpc' && <GrpcForm />}

          {network === 'xhttp' && <XhttpForm />}

          {network === 'httpupgrade' && <HttpUpgradeForm />}

          {network === 'kcp' && <KcpForm />}
        </>
      )}

      <SockoptForm toggleSockopt={toggleSockopt} network={network} />

      {/* Transport masks don't apply to tunnel (a transparent forwarder), so
          its stream tab is just sockopt + TProxy. */}
      {protocol !== Protocols.TUNNEL && (
        <Controller
          control={control}
          name="streamSettings.finalmask"
          render={({ field }) => (
            <FinalMaskField
              key={`${protocol}:${network}`}
              value={field.value}
              onChange={field.onChange}
              network={network}
              protocol={protocol}
            />
          )}
        />
      )}
    </>
  );

  const tlsOk = canEnableTls({ protocol, streamSettings: { network, security } });
  const realityOk = canEnableReality({ protocol, streamSettings: { network, security } });
  const tlsOnly = protocol === Protocols.HYSTERIA;

  const securityTab = (
    <>
      <Form.Item label={t('pages.inbounds.securityTab')}>
        <Radio.Group
          value={security}
          buttonStyle="solid"
          disabled={!tlsOk || topologyLocked}
          onChange={(e) => onSecurityChange(e.target.value)}
        >
          {!tlsOnly && <Radio.Button value="none">{t('none')}</Radio.Button>}
          <Radio.Button value="tls">TLS</Radio.Button>
          {realityOk && <Radio.Button value="reality">Reality</Radio.Button>}
        </Radio.Group>
      </Form.Item>

      {security === 'tls' && topologyLocked && (
        <>
          <FormField name={['streamSettings', 'tlsSettings', 'serverName']} label="SNI">
            <Input placeholder={t('pages.inbounds.form.serverNameIndication')} />
          </FormField>
          <PrimaryCertificateFields
            saving={saving}
            setCertFromPanel={setCertFromPanel}
            clearCertFiles={clearCertFiles}
          />
        </>
      )}

      {security === 'tls' && !topologyLocked && (
        <TlsForm
          saving={saving}
          setCertFromPanel={setCertFromPanel}
          clearCertFiles={clearCertFiles}
          pinFromCert={pinFromCert}
          pinFromRemote={pinFromRemote}
          getNewEchCert={getNewEchCert}
          clearEchCert={clearEchCert}
        />
      )}

      {security === 'reality' && (
        <RealityForm
          saving={saving}
          scanning={scanning}
          scanResult={scanResult}
          scanRealityTarget={scanRealityTarget}
          scanRealityCandidates={scanRealityCandidates}
          applyRealityScanResult={applyRealityScanResult}
          randomizeShortIds={randomizeShortIds}
          randomizeSpiderX={randomizeSpiderX}
          genRealityKeypair={genRealityKeypair}
          clearRealityKeypair={clearRealityKeypair}
          genMldsa65={genMldsa65}
          clearMldsa65={clearMldsa65}
        />
      )}
    </>
  );

  const advancedTab = (
    <div className="advanced-shell">
      <div className="advanced-panel">
        <div className="advanced-panel__header">
          <div>
            <div className="advanced-panel__title">{t('pages.inbounds.advanced.title')}</div>
            <div className="advanced-panel__subtitle">{t('pages.inbounds.advanced.subtitle')}</div>
          </div>
        </div>
        <Tabs
          className="advanced-inner-tabs"
          items={[
            {
              key: 'all',
              label: t('pages.inbounds.advanced.all'),
              children: (
                <>
                  <div className="advanced-editor-meta">
                    {t('pages.inbounds.advanced.allHelp')}
                  </div>
                  <AdvancedAllEditor streamEnabled={streamEnabled} sniffingEnabled={sniffingSupported} />
                </>
              ),
            },
            {
              key: 'settings',
              label: t('pages.inbounds.advanced.settings'),
              children: (
                <>
                  <div className="advanced-editor-meta">
                    {t('pages.inbounds.advanced.settingsHelp')}{' '}
                    <code>{'{ settings: { ... } }'}</code>.
                  </div>
                  <AdvancedSliceEditor
                    path="settings"
                    wrapKey="settings"
                    minHeight="320px"
                    maxHeight="540px"
                  />
                </>
              ),
            },
            ...(streamEnabled
              ? [{
                key: 'stream',
                label: t('pages.inbounds.advanced.stream'),
                children: (
                  <>
                    <div className="advanced-editor-meta">
                      {t('pages.inbounds.advanced.streamHelp')}{' '}
                      <code>{'{ streamSettings: { ... } }'}</code>.
                    </div>
                    <AdvancedSliceEditor
                      path="streamSettings"
                      wrapKey="streamSettings"
                      minHeight="320px"
                      maxHeight="540px"
                    />
                  </>
                ),
              }]
              : []),
            ...(sniffingSupported
              ? [{
                key: 'sniffing',
                label: t('pages.inbounds.advanced.sniffing'),
                children: (
                  <>
                    <div className="advanced-editor-meta">
                      {t('pages.inbounds.advanced.sniffingHelp')}{' '}
                      <code>{'{ sniffing: { ... } }'}</code>.
                    </div>
                    <AdvancedSliceEditor
                      path="sniffing"
                      wrapKey="sniffing"
                      minHeight="240px"
                      maxHeight="420px"
                    />
                  </>
                ),
              }]
              : []),
          ]}
        />
      </div>
    </div>
  );

  const sniffingTab = <SniffingTab />;
  const guidedTab = (
    <GuidedInboundTab
      draft={guidedDraft}
      disabled={saving}
      onTemplateChange={(template) => void onGuidedTemplateChange(template)}
      onDraftChange={setGuidedDraft}
      selectedVlessAuth={selectedVlessAuth}
      vlessAuthKind={vlessAuthKind}
      getNewVlessEnc={getNewVlessEnc}
      clearVlessEnc={clearVlessEnc}
      setCertFromPanel={setCertFromPanel}
      clearCertFiles={clearCertFiles}
      scanning={scanning}
      scanResult={scanResult}
      scanRealityTarget={scanRealityTarget}
      scanRealityCandidates={scanRealityCandidates}
      applyRealityScanResult={applyRealityScanResult}
    />
  );

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={title}
        okText={okText}
        cancelText={t('close')}
        confirmLoading={saving}
        mask={{ closable: false }}
        width={780}
        onOk={submit}
        onCancel={onClose}
        destroyOnHidden
      >
        <FormProvider {...methods}>
          <Form
            colon={false}
            labelCol={{ sm: { span: 8 } }}
            wrapperCol={{ sm: { span: 14 } }}
            labelWrap
          >
            <Tabs
              activeKey={activeTab}
              onChange={setActiveTab}
              items={[
              ...(mode === 'add'
                ? [{
                  key: 'guided',
                  label: t('pages.inbounds.guide.tab'),
                  children: guidedTab,
                  forceRender: true,
                }]
                : []),
              { key: 'basic', label: t('pages.xray.basicTemplate'), children: basicTab, forceRender: true },
              ...(([
                Protocols.VLESS,
                Protocols.SHADOWSOCKS,
                Protocols.HTTP,
                Protocols.MIXED,
                Protocols.TUNNEL,
                Protocols.TUN,
                Protocols.WIREGUARD,
                Protocols.MTPROTO,
              ] as string[]).includes(protocol) || isFallbackHost
                ? [{ key: 'protocol', label: t('pages.inbounds.protocol'), children: protocolTab, forceRender: true }]
                : []),
              ...(streamEnabled
                ? [
                  { key: 'stream', label: t('pages.inbounds.streamTab'), children: streamTab, forceRender: true },
                  ...(protocol !== Protocols.WIREGUARD && protocol !== Protocols.TUNNEL
                    ? [{ key: 'security', label: t('pages.inbounds.securityTab'), children: securityTab, forceRender: true }]
                    : []),
                ]
                : []),
              ...(sniffingSupported
                ? [{ key: 'sniffing', label: t('pages.inbounds.sniffingTab'), children: sniffingTab, forceRender: true }]
                : []),
              ...(!topologyLocked
                ? [{ key: 'advanced', label: t('pages.xray.advancedTemplate'), children: advancedTab, forceRender: true }]
                : []),
              ]}
            />
          </Form>
        </FormProvider>
      </Modal>
    </>
  );
}
