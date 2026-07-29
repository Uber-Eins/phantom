import { useTranslation } from 'react-i18next';
import { useFormContext } from 'react-hook-form';
import { Button, Collapse, Divider, Form, Input, InputNumber, Select, Space, Switch } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { FormField } from '@/components/form/rhf';
import { UTLS_FINGERPRINT } from '@/schemas/primitives';
import {
  validateRealityClientVer,
  validateRealityMaxClientVer,
} from '@/lib/xray/stream-wire-normalize';
import type { RealityScanResult } from '@/models/reality-scan';
import RealityTargetFields from './RealityTargetFields';

interface RealityFormProps {
  saving: boolean;
  scanning: boolean;
  scanResult: RealityScanResult | null;
  scanRealityTarget: () => void;
  scanRealityCandidates: (targets?: string) => Promise<RealityScanResult[]>;
  applyRealityScanResult: (result: RealityScanResult) => void;
  randomizeShortIds: () => void;
  randomizeSpiderX: () => void;
  genRealityKeypair: () => void;
  clearRealityKeypair: () => void;
  genMldsa65: () => void;
  clearMldsa65: () => void;
  syncServerName?: boolean;
}

export default function RealityForm({
  saving,
  scanning,
  scanResult,
  scanRealityTarget,
  scanRealityCandidates,
  applyRealityScanResult,
  randomizeShortIds,
  randomizeSpiderX,
  genRealityKeypair,
  clearRealityKeypair,
  genMldsa65,
  clearMldsa65,
  syncServerName = false,
}: RealityFormProps) {
  const { t } = useTranslation();
  const { getFieldState, trigger } = useFormContext();
  const maxClientVerPath = 'streamSettings.realitySettings.maxClientVer';
  const revalidateMaxClientVer = () => {
    if (getFieldState(maxClientVerPath).error) {
      void trigger(maxClientVerPath);
    }
  };
  return (
    <>
      <FormField
        name={['streamSettings', 'realitySettings', 'show']}
        label={t('pages.inbounds.form.show')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
      <FormField name={['streamSettings', 'realitySettings', 'xver']} label={t('pages.inbounds.form.xver')}>
        <InputNumber min={0} />
      </FormField>
      <FormField
        name={['streamSettings', 'realitySettings', 'settings', 'fingerprint']}
        label="uTLS"
      >
        <Select
          options={Object.values(UTLS_FINGERPRINT).map((fp) => ({ value: fp, label: fp }))}
        />
      </FormField>
      <RealityTargetFields
        scanning={scanning}
        scanResult={scanResult}
        scanRealityTarget={scanRealityTarget}
        scanRealityCandidates={scanRealityCandidates}
        applyRealityScanResult={applyRealityScanResult}
        syncServerName={syncServerName}
      />
      <FormField
        name={['streamSettings', 'realitySettings', 'maxTimediff']}
        label={t('pages.inbounds.form.maxTimeDiff')}
      >
        <InputNumber min={0} />
      </FormField>
      <FormField
        name={['streamSettings', 'realitySettings', 'minClientVer']}
        label={t('pages.inbounds.form.minClientVer')}
        tooltip={t('pages.inbounds.form.minClientVerHint')}
        onAfterChange={revalidateMaxClientVer}
        rules={{
          validate: (value) => {
            const errKey = validateRealityClientVer(typeof value === 'string' ? value : '');
            return errKey ? errKey : true;
          },
        }}
      >
        <Input placeholder="26.3.27" />
      </FormField>
      <FormField
        name={['streamSettings', 'realitySettings', 'maxClientVer']}
        label={t('pages.inbounds.form.maxClientVer')}
        tooltip={t('pages.inbounds.form.maxClientVerHint')}
        rules={{
          validate: (value, formValues) => {
            const max = typeof value === 'string' ? value : '';
            const min = formValues?.streamSettings?.realitySettings?.minClientVer;
            const errKey = validateRealityMaxClientVer(max, typeof min === 'string' ? min : '');
            return errKey ? errKey : true;
          },
        }}
      >
        <Input placeholder="x.y.z" />
      </FormField>
      <Form.Item label={t('pages.inbounds.form.shortIds')}>
        <Space.Compact block style={{ display: 'flex' }}>
          <FormField
            name={['streamSettings', 'realitySettings', 'shortIds']}
            noStyle
          >
            <Select mode="tags" tokenSeparators={[',']} style={{ flex: 1 }} />
          </FormField>
          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={randomizeShortIds} />
        </Space.Compact>
      </Form.Item>
      <Form.Item
        label={t('pages.inbounds.form.spiderX')}
        tooltip={t('pages.inbounds.form.spiderXHint')}
      >
        <Space.Compact block style={{ display: 'flex' }}>
          <FormField
            name={['streamSettings', 'realitySettings', 'settings', 'spiderX']}
            noStyle
          >
            <Input style={{ flex: 1 }} />
          </FormField>
          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={randomizeSpiderX} />
        </Space.Compact>
      </Form.Item>
      <FormField
        name={['streamSettings', 'realitySettings', 'settings', 'publicKey']}
        label={t('pages.inbounds.publicKey')}
      >
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </FormField>
      <FormField
        name={['streamSettings', 'realitySettings', 'privateKey']}
        label={t('pages.inbounds.privatekey')}
      >
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </FormField>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={saving} onClick={genRealityKeypair}>
            {t('pages.inbounds.form.getNewCert')}
          </Button>
          <Button danger onClick={clearRealityKeypair}>{t('clear')}</Button>
        </Space>
      </Form.Item>
      <FormField
        name={['streamSettings', 'realitySettings', 'mldsa65Seed']}
        label={t('pages.inbounds.form.mldsa65Seed')}
      >
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </FormField>
      <FormField
        name={['streamSettings', 'realitySettings', 'settings', 'mldsa65Verify']}
        label={t('pages.inbounds.form.mldsa65Verify')}
      >
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </FormField>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={saving} onClick={genMldsa65}>
            {t('pages.inbounds.form.getNewSeed')}
          </Button>
          <Button danger onClick={clearMldsa65}>{t('clear')}</Button>
        </Space>
      </Form.Item>
      <FormField
        name={['streamSettings', 'realitySettings', 'masterKeyLog']}
        label={t('pages.inbounds.form.masterKeyLog')}
        tooltip={t('pages.inbounds.form.masterKeyLogTip')}
      >
        <Input placeholder="/path/to/sslkeylog.txt" />
      </FormField>
      <Collapse
        style={{ marginBottom: 14 }}
        items={[
          {
            key: 'limitFallback',
            label: t('pages.inbounds.form.limitFallback'),
            children: (
              <>
                {(['limitFallbackUpload', 'limitFallbackDownload'] as const).map((dir) => (
                  <div key={dir}>
                    <Divider style={{ margin: '0 0 14px 0' }}>
                      {t(`pages.inbounds.form.${dir}`)}
                    </Divider>
                    <FormField
                      name={['streamSettings', 'realitySettings', dir, 'afterBytes']}
                      label={t('pages.inbounds.form.afterBytes')}
                      tooltip={t('pages.inbounds.form.afterBytesTip')}
                    >
                      <InputNumber min={0} />
                    </FormField>
                    <FormField
                      name={['streamSettings', 'realitySettings', dir, 'bytesPerSec']}
                      label={t('pages.inbounds.form.bytesPerSec')}
                      tooltip={t('pages.inbounds.form.bytesPerSecTip')}
                    >
                      <InputNumber min={0} />
                    </FormField>
                    <FormField
                      name={['streamSettings', 'realitySettings', dir, 'burstBytesPerSec']}
                      label={t('pages.inbounds.form.burstBytesPerSec')}
                      tooltip={t('pages.inbounds.form.burstBytesPerSecTip')}
                    >
                      <InputNumber min={0} />
                    </FormField>
                  </div>
                ))}
              </>
            ),
          },
        ]}
      />
    </>
  );
}
