import { useEffect } from 'react';
import { FormProvider } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Typography,
} from 'antd';

import { useIssueCertificate } from '@/api/queries/useIssueCertificate';
import { FormField, useZodForm } from '@/components/form/rhf';
import { SettingListItem } from '@/components/ui';
import {
  CertificateIssueRequestSchema,
  type CertificateIssueRequest,
} from '@/schemas/certificate';

interface CertificateFormModalProps {
  open: boolean;
  defaultEmail?: string;
  onClose: () => void;
}

function defaultValues(defaultEmail = ''): CertificateIssueRequest {
  return {
    remark: '',
    addMethod: 'acme',
    ca: 'zerossl',
    validationMethod: 'cloudflare',
    cloudflareToken: '',
    identifiers: '',
    email: defaultEmail,
    keyType: 'RSA2048',
    certificateType: 'domain',
  };
}

export default function CertificateFormModal({
  open,
  defaultEmail,
  onClose,
}: CertificateFormModalProps) {
  const { t } = useTranslation();
  const issueCertificate = useIssueCertificate();
  const methods = useZodForm(CertificateIssueRequestSchema, {
    defaultValues: defaultValues(defaultEmail),
  });

  useEffect(() => {
    if (open) methods.reset(defaultValues(defaultEmail));
  }, [defaultEmail, methods, open]);

  async function onSubmit(values: CertificateIssueRequest) {
    const message = await issueCertificate.mutateAsync(values);
    if (message.success) onClose();
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      title={t('pages.certificates.addCertificate')}
      footer={null}
      width={760}
      destroyOnHidden
      className="certificate-form-modal"
    >
      <FormProvider {...methods}>
        <Form
          layout="vertical"
          className="certificate-form"
          onFinish={methods.handleSubmit(onSubmit)}
        >
          <SettingListItem paddings="small" title={t('pages.certificates.remark')}>
            <FormField name="remark" style={{ marginBottom: 0 }}>
              <Input
                id="certificate-remark"
                aria-label={t('pages.certificates.remark')}
                placeholder={t('pages.certificates.remarkPlaceholder')}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.certificates.addMethod')}>
            <FormField name="addMethod" style={{ marginBottom: 0 }}>
              <Select
                id="certificate-add-method"
                aria-label={t('pages.certificates.addMethod')}
                options={[{ value: 'acme', label: 'ACME' }]}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.certificates.authority')}>
            <FormField name="ca" style={{ marginBottom: 0 }}>
              <Select
                id="certificate-authority"
                aria-label={t('pages.certificates.authority')}
                options={[
                  { value: 'zerossl', label: 'ZeroSSL' },
                  { value: 'letsencrypt', label: "Let's Encrypt" },
                ]}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.certificates.validationMethod')}>
            <FormField name="validationMethod" style={{ marginBottom: 0 }}>
              <Select
                id="certificate-validation-method"
                aria-label={t('pages.certificates.validationMethod')}
                options={[{ value: 'cloudflare', label: 'Cloudflare' }]}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem
            paddings="small"
            title={t('pages.certificates.cloudflareToken')}
            description={(
              <Space orientation="vertical" size={2}>
                <Typography.Link
                  href="https://dash.cloudflare.com/profile/api-tokens"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {t('pages.certificates.createCloudflareToken')}
                </Typography.Link>
                <Typography.Text type="danger">
                  {t('pages.certificates.globalTokenWarning')}
                </Typography.Text>
              </Space>
            )}
          >
            <FormField name="cloudflareToken" style={{ marginBottom: 0 }}>
              <Input.Password
                id="certificate-cloudflare-token"
                aria-label={t('pages.certificates.cloudflareToken')}
                autoComplete="off"
              />
            </FormField>
          </SettingListItem>

          <SettingListItem
            paddings="small"
            title={t('pages.certificates.identifiers')}
            description={t('pages.certificates.identifiersDescription')}
          >
            <FormField name="identifiers" style={{ marginBottom: 0 }}>
              <Input.TextArea
                id="certificate-identifiers"
                aria-label={t('pages.certificates.identifiers')}
                autoSize={{ minRows: 3, maxRows: 8 }}
                placeholder={t('pages.certificates.identifiersPlaceholder')}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.certificates.email')}>
            <FormField name="email" style={{ marginBottom: 0 }}>
              <Input
                id="certificate-email"
                aria-label={t('pages.certificates.email')}
                type="email"
                autoComplete="email"
                placeholder={t('pages.certificates.emailPlaceholder')}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.certificates.keyType')}>
            <FormField name="keyType" style={{ marginBottom: 0 }}>
              <Select
                id="certificate-key-type"
                aria-label={t('pages.certificates.keyType')}
                options={['EC256', 'EC384', 'RSA2048', 'RSA4096'].map((value) => ({
                  value,
                  label: value,
                }))}
              />
            </FormField>
          </SettingListItem>

          <SettingListItem
            paddings="small"
            title={t('pages.certificates.certificateType')}
            description={t('pages.certificates.ipCertificateHint')}
          >
            <FormField name="certificateType" style={{ marginBottom: 0 }}>
              <Select
                id="certificate-type"
                aria-label={t('pages.certificates.certificateType')}
                options={[
                  { value: 'domain', label: t('pages.certificates.domainCertificate') },
                  {
                    value: 'ip',
                    label: t('pages.certificates.ipCertificate'),
                    disabled: true,
                  },
                ]}
              />
            </FormField>
          </SettingListItem>

          <div className="certificate-actions">
            <Button onClick={onClose}>{t('cancel')}</Button>
            <Button
              type="primary"
              htmlType="submit"
              loading={issueCertificate.isPending}
            >
              {t('pages.certificates.issue')}
            </Button>
          </div>
        </Form>
      </FormProvider>
    </Modal>
  );
}
