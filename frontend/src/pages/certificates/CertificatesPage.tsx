import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Col,
  ConfigProvider,
  Layout,
  Result,
  Row,
  Spin,
  message,
} from 'antd';

import {
  useCertificateConfig,
  useCertificates,
} from '@/api/queries/useCertificates';
import { useDeleteCertificate } from '@/api/queries/useIssueCertificate';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import type {
  CertificateConfig,
  CertificateRecord,
} from '@/schemas/certificate';
import { setMessageInstance } from '@/utils/messageBus';

import CertificateConfigModal from './CertificateConfigModal';
import CertificateFormModal from './CertificateFormModal';
import CertificateList from './CertificateList';
import './CertificatesPage.css';

export default function CertificatesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const certificates = useCertificates();
  const certificateConfig = useCertificateConfig();
  const deleteCertificate = useDeleteCertificate();
  const [formOpen, setFormOpen] = useState(false);
  const [editingCertificate, setEditingCertificate] = useState<CertificateRecord | null>(null);
  const [configOpen, setConfigOpen] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  async function saveConfig(config: CertificateConfig) {
    const result = await certificateConfig.save.mutateAsync(config);
    return result.success;
  }

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout
        className={`inbounds-page certificates-page${isDark ? ' is-dark' : ''}${isUltra ? ' is-ultra' : ''}`}
      >
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin
              spinning={certificates.isLoading || certificateConfig.isLoading}
              delay={200}
              description={t('loading')}
              size="large"
            >
              {certificates.error || certificateConfig.error ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={String(certificates.error || certificateConfig.error)}
                  extra={(
                    <Button
                      type="primary"
                      onClick={() => {
                        certificates.refetch();
                        certificateConfig.refetch();
                      }}
                    >
                      {t('refresh')}
                    </Button>
                  )}
                />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, 12]}>
                  <Col span={24}>
                    <CertificateList
                      certificates={certificates.data ?? []}
                      isMobile={isMobile}
                      onAdd={() => {
                        setEditingCertificate(null);
                        setFormOpen(true);
                      }}
                      onConfig={() => setConfigOpen(true)}
                      onEdit={(certificate) => {
                        setEditingCertificate(certificate);
                        setFormOpen(true);
                      }}
                      onDelete={async (certificate) => {
                        await deleteCertificate.mutateAsync(certificate.id);
                      }}
                    />
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <CertificateFormModal
          open={formOpen}
          defaultEmail={certificateConfig.data?.defaultEmail}
          certificate={editingCertificate}
          onClose={() => {
            setFormOpen(false);
            setEditingCertificate(null);
          }}
        />
        <CertificateConfigModal
          open={configOpen}
          config={certificateConfig.data}
          saving={certificateConfig.save.isPending}
          onClose={() => setConfigOpen(false)}
          onSave={saveConfig}
        />
      </Layout>
    </ConfigProvider>
  );
}
