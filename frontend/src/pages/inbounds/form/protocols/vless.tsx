import { useTranslation } from 'react-i18next';
import { Form, InputNumber, Space } from 'antd';
import { Controller, useFormContext } from 'react-hook-form';

import type { VlessAuthKind } from '@/lib/xray/vless-encryption';
import VlessEncFields from './VlessEncFields';

interface VlessFieldsProps {
  saving: boolean;
  selectedVlessAuth: string;
  vlessAuthKind: VlessAuthKind | null;
  network: string;
  security: string;
  getNewVlessEnc: (kind: VlessAuthKind) => void;
  clearVlessEnc: () => void;
}

export default function VlessFields({
  saving,
  selectedVlessAuth,
  vlessAuthKind,
  network,
  security,
  getNewVlessEnc,
  clearVlessEnc,
}: VlessFieldsProps) {
  const { t } = useTranslation();
  const { control } = useFormContext();

  return (
    <>
      <VlessEncFields
        saving={saving}
        selectedVlessAuth={selectedVlessAuth}
        vlessAuthKind={vlessAuthKind}
        getNewVlessEnc={getNewVlessEnc}
        clearVlessEnc={clearVlessEnc}
      />
      {network === 'tcp' && (security === 'tls' || security === 'reality') && (
        <Form.Item
          label={t('pages.inbounds.form.visionTestseed')}
          extra="Applies only to clients using the xtls-rprx-vision flow; ignored otherwise."
        >
          <Space.Compact block>
            {[900, 500, 900, 256].map((def, i) => (
              <Controller
                key={i}
                control={control}
                name={`settings.testseed.${i}`}
                defaultValue={def}
                render={({ field }) => (
                  <InputNumber
                    min={1}
                    style={{ width: '25%' }}
                    value={field.value as number}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                )}
              />
            ))}
          </Space.Compact>
        </Form.Item>
      )}
    </>
  );
}
