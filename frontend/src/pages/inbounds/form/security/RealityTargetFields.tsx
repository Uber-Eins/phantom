import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useFormContext } from 'react-hook-form';
import {
  Alert,
  Button,
  Descriptions,
  Form,
  Input,
  Select,
  Space,
} from 'antd';
import { RadarChartOutlined, SearchOutlined } from '@ant-design/icons';

import { FormField } from '@/components/form/rhf';
import {
  realityTargetServerName,
  validateRealityTarget,
} from '@/lib/xray/stream-wire-normalize';
import type { RealityScanResult } from '@/models/reality-scan';
import type { InboundFormValues } from '@/schemas/forms/inbound-form';

import RealityTargetScannerModal from './RealityTargetScannerModal';

export interface RealityTargetFieldsProps {
  scanning: boolean;
  scanResult: RealityScanResult | null;
  scanRealityTarget: () => void;
  scanRealityCandidates: (targets?: string) => Promise<RealityScanResult[]>;
  applyRealityScanResult: (result: RealityScanResult) => void;
  syncServerName?: boolean;
}

export default function RealityTargetFields({
  scanning,
  scanResult,
  scanRealityTarget,
  scanRealityCandidates,
  applyRealityScanResult,
  syncServerName = false,
}: RealityTargetFieldsProps) {
  const { t } = useTranslation();
  const [scannerOpen, setScannerOpen] = useState(false);
  const { setValue } = useFormContext<InboundFormValues>();

  return (
    <>
      <Form.Item
        label={t('pages.inbounds.form.target')}
        tooltip={t('pages.inbounds.form.realityTargetHint')}
      >
        <Space.Compact block style={{ display: 'flex' }}>
          <FormField
            name={['streamSettings', 'realitySettings', 'target']}
            noStyle
            rules={{
              validate: (value) => {
                const errKey = validateRealityTarget(typeof value === 'string' ? value : '');
                return errKey ? errKey : true;
              },
            }}
            onAfterChange={(value) => {
              const serverName = realityTargetServerName(
                typeof value === 'string' ? value : '',
              );
              if (syncServerName && serverName) {
                setValue(
                  'streamSettings.realitySettings.serverNames',
                  [serverName],
                  { shouldDirty: true, shouldValidate: true },
                );
              }
            }}
          >
            <Input style={{ flex: 1 }} placeholder="example.com:443" />
          </FormField>
          <Button icon={<RadarChartOutlined />} loading={scanning} onClick={scanRealityTarget}>
            {t('pages.inbounds.form.scan')}
          </Button>
          <Button icon={<SearchOutlined />} onClick={() => setScannerOpen(true)}>
            {t('pages.inbounds.form.findTargets')}
          </Button>
        </Space.Compact>
      </Form.Item>
      {scanResult && (
        <Form.Item label=" " colon={false}>
          <Alert
            type={scanResult.feasible ? 'success' : 'warning'}
            showIcon
            title={
              scanResult.feasible
                ? t('pages.inbounds.form.scanFeasible')
                : scanResult.reason || t('pages.inbounds.form.scanNotFeasible')
            }
            description={
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="TLS">{scanResult.tlsVersion || '—'}</Descriptions.Item>
                <Descriptions.Item label="ALPN">{scanResult.alpn || '—'}</Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanCurve')}>
                  {scanResult.curveID || '—'}
                </Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanCert')}>
                  {scanResult.certValid
                    ? `${scanResult.certSubject} (${scanResult.certIssuer})`
                    : t('pages.inbounds.form.scanCertInvalid')}
                </Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanLatency')}>
                  {scanResult.latencyMs > 0 ? `${scanResult.latencyMs} ms` : '—'}
                </Descriptions.Item>
              </Descriptions>
            }
          />
        </Form.Item>
      )}
      <FormField label="SNI" name={['streamSettings', 'realitySettings', 'serverNames']}>
        <Select mode="tags" tokenSeparators={[',']} style={{ width: '100%' }} />
      </FormField>
      <RealityTargetScannerModal
        open={scannerOpen}
        onClose={() => setScannerOpen(false)}
        scanRealityCandidates={scanRealityCandidates}
        onPick={applyRealityScanResult}
      />
    </>
  );
}
