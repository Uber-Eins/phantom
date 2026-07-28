import { useEffect } from 'react';
import { FormProvider } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, InputNumber, Modal, Select } from 'antd';

import { FormField, useZodForm } from '@/components/form/rhf';
import { SettingListItem } from '@/components/ui';
import {
  CertificateConfigSchema,
  type CertificateConfig,
} from '@/schemas/certificate';

interface CertificateConfigModalProps {
  open: boolean;
  config: CertificateConfig | null | undefined;
  saving: boolean;
  onClose: () => void;
  onSave: (config: CertificateConfig) => Promise<boolean>;
}

const defaultConfig: CertificateConfig = {
  renewBeforeDays: 30,
  shortRenewBeforeHours: 24,
  shortCheckTimesPerDay: 4,
  checkTime: '05:00:00',
  defaultEmail: '',
  globalPrivateKey: '',
};

export default function CertificateConfigModal({
  open,
  config,
  saving,
  onClose,
  onSave,
}: CertificateConfigModalProps) {
  const { t } = useTranslation();
  const methods = useZodForm(CertificateConfigSchema, {
    defaultValues: config ?? defaultConfig,
  });

  useEffect(() => {
    if (open && config) methods.reset(config);
  }, [config, methods, open]);

  async function onSubmit(values: CertificateConfig) {
    if (await onSave(values)) onClose();
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      title={t('pages.certificates.generalConfig')}
      footer={null}
      width={760}
      destroyOnHidden
      className="certificate-config-modal"
    >
      <FormProvider {...methods}>
        <Form layout="vertical" onFinish={methods.handleSubmit(onSubmit)}>
          <div className="certificate-config-panel">
            <SettingListItem
              paddings="small"
              title={t('pages.certificates.renewBeforeDays')}
              description={t('pages.certificates.renewBeforeDaysRange')}
            >
              <FormField name="renewBeforeDays" style={{ marginBottom: 0 }}>
                <InputNumber
                  id="certificate-renew-before-days"
                  aria-label={t('pages.certificates.renewBeforeDays')}
                  min={1}
                  max={90}
                  precision={0}
                  style={{ width: '100%' }}
                />
              </FormField>
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.certificates.shortRenewBeforeHours')}
              description={t('pages.certificates.shortRenewBeforeHoursRange')}
            >
              <FormField name="shortRenewBeforeHours" style={{ marginBottom: 0 }}>
                <InputNumber
                  id="certificate-short-renew-before-hours"
                  aria-label={t('pages.certificates.shortRenewBeforeHours')}
                  min={24}
                  max={168}
                  precision={0}
                  style={{ width: '100%' }}
                />
              </FormField>
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.certificates.shortCheckTimesPerDay')}
              description={t('pages.certificates.shortCheckTimesHint')}
            >
              <FormField name="shortCheckTimesPerDay" style={{ marginBottom: 0 }}>
                <Select
                  id="certificate-short-check-times"
                  aria-label={t('pages.certificates.shortCheckTimesPerDay')}
                  options={[4, 5, 6].map((count) => ({
                    value: count,
                    label: t('pages.certificates.timesPerDay', { count }),
                  }))}
                />
              </FormField>
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.certificates.checkTime')}
              description={t('pages.certificates.checkTimeHint')}
            >
              <FormField name="checkTime" style={{ marginBottom: 0 }}>
                <Input
                  id="certificate-check-time"
                  aria-label={t('pages.certificates.checkTime')}
                  type="time"
                  step={1}
                />
              </FormField>
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.certificates.defaultEmail')}>
              <FormField name="defaultEmail" style={{ marginBottom: 0 }}>
                <Input
                  id="certificate-default-email"
                  aria-label={t('pages.certificates.defaultEmail')}
                  type="email"
                  autoComplete="email"
                />
              </FormField>
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.certificates.globalPrivateKey')}
            >
              <FormField name="globalPrivateKey" style={{ marginBottom: 0 }}>
                <Input.TextArea
                  id="certificate-global-private-key"
                  aria-label={t('pages.certificates.globalPrivateKey')}
                  autoSize={{ minRows: 6, maxRows: 12 }}
                  spellCheck={false}
                />
              </FormField>
            </SettingListItem>
          </div>

          <div className="certificate-actions">
            <Button onClick={onClose}>{t('cancel')}</Button>
            <Button type="primary" htmlType="submit" loading={saving}>
              {t('save')}
            </Button>
          </div>
        </Form>
      </FormProvider>
    </Modal>
  );
}
