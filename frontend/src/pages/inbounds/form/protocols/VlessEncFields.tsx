import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Select, Space, Typography } from 'antd';

import { FormField } from '@/components/form/rhf';
import { VLESS_AUTH_LABEL_KEYS, type VlessAuthKind } from '@/lib/xray/vless-encryption';

export interface VlessEncFieldsProps {
  saving: boolean;
  selectedVlessAuth: string;
  vlessAuthKind: VlessAuthKind | null;
  getNewVlessEnc: (kind: VlessAuthKind) => void;
  clearVlessEnc: () => void;
}

export default function VlessEncFields({
  saving,
  selectedVlessAuth,
  vlessAuthKind,
  getNewVlessEnc,
  clearVlessEnc,
}: VlessEncFieldsProps) {
  const { t } = useTranslation();
  const [authKind, setAuthKind] = useState<VlessAuthKind>(vlessAuthKind ?? 'x25519');

  useEffect(() => {
    setAuthKind(vlessAuthKind ?? 'x25519');
  }, [vlessAuthKind]);

  const authOptions = (Object.entries(VLESS_AUTH_LABEL_KEYS) as [VlessAuthKind, string][]).map(
    ([value, labelKey]) => ({ value, label: t(labelKey) }),
  );

  return (
    <>
      <FormField name={['settings', 'decryption']} label={t('pages.inbounds.decryption')}>
        <Input />
      </FormField>
      <FormField name={['settings', 'encryption']} label={t('pages.inbounds.encryption')}>
        <Input />
      </FormField>
      <Form.Item label={t('pages.inbounds.vlessAuthGenerate')}>
        <Space size={8} wrap>
          <Select
            value={authKind}
            onChange={setAuthKind}
            options={authOptions}
            style={{ width: 240 }}
          />
          <Button type="primary" loading={saving} onClick={() => getNewVlessEnc(authKind)}>
            {t('pages.inbounds.vlessAuthGenerateButton')}
          </Button>
          <Button danger onClick={clearVlessEnc}>{t('clear')}</Button>
        </Space>
        <Typography.Text type="secondary" className="vless-auth-state">
          {t('pages.inbounds.vlessAuthSelected', { auth: selectedVlessAuth })}
        </Typography.Text>
      </Form.Item>
    </>
  );
}
