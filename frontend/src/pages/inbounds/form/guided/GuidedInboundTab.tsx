import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, Select } from 'antd';

import { FormField } from '@/components/form/rhf';
import type { VlessAuthKind } from '@/lib/xray/vless-encryption';
import type { RealityScanResult } from '@/models/reality-scan';
import VlessEncFields from '../protocols/VlessEncFields';
import { PrimaryCertificateFields, RealityTargetFields } from '../security';

import { GUIDED_TEMPLATE_OPTIONS } from './templates';
import type { GuidedDecoyMode, GuidedDraft, GuidedTemplate } from './types';

interface GuidedInboundTabProps {
  draft: GuidedDraft | null;
  disabled: boolean;
  onTemplateChange: (template?: GuidedTemplate) => void;
  onDraftChange: (draft: GuidedDraft) => void;
  selectedVlessAuth: string;
  vlessAuthKind: VlessAuthKind | null;
  getNewVlessEnc: (kind: VlessAuthKind) => void;
  clearVlessEnc: () => void;
  setCertFromPanel: (certName: number) => void;
  clearCertFiles: (certName: number) => void;
  scanning: boolean;
  scanResult: RealityScanResult | null;
  scanRealityTarget: () => void;
  scanRealityCandidates: (targets?: string) => Promise<RealityScanResult[]>;
  applyRealityScanResult: (result: RealityScanResult) => void;
}

const TLS_DECOY_OPTIONS: Array<{ value: GuidedDecoyMode; labelKey: string }> = [
  { value: 'unauthorized', labelKey: 'pages.inbounds.guide.decoyUnauthorized' },
  { value: 'proxy', labelKey: 'pages.inbounds.guide.decoyProxy' },
  { value: 'static', labelKey: 'pages.inbounds.guide.decoyStatic' },
];

export default function GuidedInboundTab({
  draft,
  disabled,
  onTemplateChange,
  onDraftChange,
  selectedVlessAuth,
  vlessAuthKind,
  getNewVlessEnc,
  clearVlessEnc,
  setCertFromPanel,
  clearCertFiles,
  scanning,
  scanResult,
  scanRealityTarget,
  scanRealityCandidates,
  applyRealityScanResult,
}: GuidedInboundTabProps) {
  const { t } = useTranslation();

  return (
    <>
      <Alert
        className="mb-12"
        type="info"
        showIcon
        title={t('pages.inbounds.guide.summary')}
      />
      <Form.Item label={t('pages.inbounds.guide.template')} required>
        <Select<GuidedTemplate>
          id="guided-template"
          allowClear
          loading={disabled}
          disabled={disabled}
          value={draft?.template}
          placeholder={t('pages.inbounds.guide.templatePlaceholder')}
          options={GUIDED_TEMPLATE_OPTIONS}
          onChange={(value) => onTemplateChange(value)}
          onClear={() => onTemplateChange(undefined)}
        />
      </Form.Item>

      {draft && (
        <VlessEncFields
          saving={disabled}
          selectedVlessAuth={selectedVlessAuth}
          vlessAuthKind={vlessAuthKind}
          getNewVlessEnc={getNewVlessEnc}
          clearVlessEnc={clearVlessEnc}
        />
      )}

      {draft?.security === 'tls' && (
        <>
          <FormField name={['streamSettings', 'tlsSettings', 'serverName']} label="SNI">
            <Input placeholder="example.com" />
          </FormField>
          <PrimaryCertificateFields
            saving={disabled}
            setCertFromPanel={setCertFromPanel}
            clearCertFiles={clearCertFiles}
          />
          <Form.Item label={t('pages.inbounds.guide.decoy')}>
            <Select<GuidedDecoyMode>
              value={draft.decoyMode}
              options={TLS_DECOY_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onChange={(decoyMode) => onDraftChange({
                ...draft,
                decoyMode,
                decoyValue: '',
              })}
            />
          </Form.Item>
          {draft.decoyMode === 'proxy' && (
            <Form.Item label={t('pages.inbounds.guide.proxyTarget')} required>
              <Input
                value={draft.decoyValue}
                placeholder="https://example.com"
                onChange={(event) => onDraftChange({ ...draft, decoyValue: event.target.value })}
              />
            </Form.Item>
          )}
          {draft.decoyMode === 'static' && (
            <>
              <Form.Item label={t('pages.inbounds.guide.staticPath')} required>
                <Input
                  value={draft.decoyValue}
                  placeholder="/var/www/site"
                  onChange={(event) => onDraftChange({ ...draft, decoyValue: event.target.value })}
                />
              </Form.Item>
              <Alert
                type="warning"
                showIcon
                title={t('pages.inbounds.guide.staticWarning')}
              />
            </>
          )}
        </>
      )}

      {draft?.security === 'reality' && (
        <>
          <RealityTargetFields
            scanning={scanning}
            scanResult={scanResult}
            scanRealityTarget={scanRealityTarget}
            scanRealityCandidates={scanRealityCandidates}
            applyRealityScanResult={applyRealityScanResult}
            syncServerName
          />
          <Alert
            type="info"
            showIcon
            title={t('pages.inbounds.guide.realityDecoy')}
          />
        </>
      )}
    </>
  );
}
