import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Input,
  Popconfirm,
  Space,
  Table,
  Tag,
  type TableColumnType,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  MenuOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from '@ant-design/icons';

import { useDatepicker } from '@/hooks/useDatepicker';
import type { CertificateRecord } from '@/schemas/certificate';
import { IntlUtil } from '@/utils';

interface CertificateListProps {
  certificates: CertificateRecord[];
  isMobile: boolean;
  onAdd: () => void;
  onConfig: () => void;
  onEdit: (certificate: CertificateRecord) => void;
  onDelete: (certificate: CertificateRecord) => void | Promise<void>;
}

export default function CertificateList({
  certificates,
  isMobile,
  onAdd,
  onConfig,
  onEdit,
  onDelete,
}: CertificateListProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const [searchKey, setSearchKey] = useState('');

  const visibleCertificates = useMemo(() => {
    const query = searchKey.trim().toLowerCase();
    if (!query) return certificates;
    return certificates.filter((certificate) => (
      certificate.remark.toLowerCase().includes(query)
      || certificate.certificateIdentifiers.toLowerCase().includes(query)
      || certificate.ca.toLowerCase().includes(query)
      || certificate.validationMethod.toLowerCase().includes(query)
    ));
  }, [certificates, searchKey]);

  const columns = useMemo<TableColumnType<CertificateRecord>[]>(() => [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 65,
      align: 'right',
      sorter: (first, second) => first.id - second.id,
    },
    {
      title: t('pages.certificates.remark'),
      dataIndex: 'remark',
      key: 'remark',
      width: 150,
      sorter: (first, second) => first.remark.localeCompare(second.remark),
    },
    {
      title: t('pages.certificates.issuedAt'),
      dataIndex: 'issuedAt',
      key: 'issuedAt',
      width: 190,
      sorter: (first, second) => Date.parse(first.issuedAt) - Date.parse(second.issuedAt),
      render: (value: string) => IntlUtil.formatDate(value, datepicker),
    },
    {
      title: t('pages.certificates.expiresAt'),
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      width: 190,
      sorter: (first, second) => Date.parse(first.expiresAt) - Date.parse(second.expiresAt),
      render: (value: string) => IntlUtil.formatDate(value, datepicker),
    },
    {
      title: t('pages.certificates.identifiers'),
      dataIndex: 'certificateIdentifiers',
      key: 'identifiers',
      width: 260,
      render: (value: string) => (
        <div className="certificate-identifiers">
          {value.split('\n').filter(Boolean).join(', ')}
        </div>
      ),
    },
    {
      title: t('pages.certificates.authority'),
      dataIndex: 'ca',
      key: 'ca',
      width: 135,
      render: (value: CertificateRecord['ca']) => (
        <Tag color="blue">{value === 'zerossl' ? 'ZeroSSL' : "Let's Encrypt"}</Tag>
      ),
    },
    {
      title: t('pages.certificates.validationMethod'),
      dataIndex: 'validationMethod',
      key: 'validationMethod',
      width: 125,
      render: () => <Tag color="green">Cloudflare</Tag>,
    },
    {
      title: '',
      key: 'actions',
      width: 90,
      fixed: 'right',
      render: (_value, certificate) => (
        <Space size={0}>
          <Button
            type="text"
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => onEdit(certificate)}
          />
          <Popconfirm
            title={`${t('delete')}?`}
            okText={t('delete')}
            cancelText={t('cancel')}
            onConfirm={() => onDelete(certificate)}
          >
            <Button
              type="text"
              size="small"
              danger
              icon={<DeleteOutlined />}
              aria-label={t('delete')}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ], [datepicker, onDelete, onEdit, t]);

  return (
    <Card
      hoverable
      title={(
        <Space wrap>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={onAdd}
            aria-label={t('pages.certificates.addCertificate')}
          >
            {!isMobile && t('pages.certificates.addCertificate')}
          </Button>
          <Button
            type="primary"
            icon={<MenuOutlined />}
            onClick={onConfig}
            aria-label={t('pages.certificates.generalConfig')}
          >
            {!isMobile && t('pages.certificates.generalConfig')}
          </Button>
          <Input
            value={searchKey}
            onChange={(event) => setSearchKey(event.target.value)}
            placeholder={t('search')}
            allowClear
            prefix={<SearchOutlined />}
            style={{ maxWidth: isMobile ? 140 : 220 }}
            aria-label={t('search')}
          />
        </Space>
      )}
    >
      {isMobile ? (
        <div className="certificate-cards">
          {visibleCertificates.length === 0 ? (
            <div className="card-empty">
              <SafetyCertificateOutlined style={{ fontSize: 28, opacity: 0.5 }} />
              <div>{t('noData')}</div>
            </div>
          ) : visibleCertificates.map((certificate) => (
            <div className="certificate-card" key={certificate.id}>
              <div className="certificate-card-head">
                <span className="certificate-card-id">#{certificate.id}</span>
                <strong>{certificate.remark}</strong>
              </div>
              <div>
                {certificate.certificateIdentifiers.split('\n').filter(Boolean).join(', ')}
              </div>
              <div className="certificate-card-dates">
                <span>{t('pages.certificates.issuedAt')}</span>
                <span>{IntlUtil.formatDate(certificate.issuedAt, datepicker)}</span>
                <span>{t('pages.certificates.expiresAt')}</span>
                <span>{IntlUtil.formatDate(certificate.expiresAt, datepicker)}</span>
              </div>
              <Space size={4} wrap>
                <Tag color="blue">
                  {certificate.ca === 'zerossl' ? 'ZeroSSL' : "Let's Encrypt"}
                </Tag>
                <Tag color="green">Cloudflare</Tag>
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  aria-label={t('edit')}
                  onClick={() => onEdit(certificate)}
                />
                <Popconfirm
                  title={`${t('delete')}?`}
                  okText={t('delete')}
                  cancelText={t('cancel')}
                  onConfirm={() => onDelete(certificate)}
                >
                  <Button
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                    aria-label={t('delete')}
                  />
                </Popconfirm>
              </Space>
            </div>
          ))}
        </div>
      ) : (
        <Table
          columns={columns}
          dataSource={visibleCertificates}
          rowKey="id"
          size="small"
          pagination={{
            pageSize: 25,
            showSizeChanger: false,
            hideOnSinglePage: true,
          }}
          scroll={{ x: columns.reduce((sum, column) => sum + Number(column.width || 0), 0) }}
          locale={{
            emptyText: (
              <div className="card-empty">
                <SafetyCertificateOutlined style={{ fontSize: 32, marginBottom: 8 }} />
                <div>{t('noData')}</div>
              </div>
            ),
          }}
        />
      )}
    </Card>
  );
}
