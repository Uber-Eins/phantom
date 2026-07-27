import { lazy, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Modal, Select, Spin, message } from 'antd';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';

import { ClipboardManager, FileManager, HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';

const JsonEditor = lazy(() => import('@/components/form/JsonEditor'));

type ConfigSource = 'xray-core' | 'nginx';

interface ConfigModalProps {
  open: boolean;
  onClose: () => void;
}

const configSources: Record<
  ConfigSource,
  { endpoint: string; fileName: string; language: 'json' | 'text' }
> = {
  'xray-core': {
    endpoint: '/panel/api/server/getConfigJson',
    fileName: 'config.json',
    language: 'json',
  },
  nginx: {
    endpoint: '/panel/api/server/getNginxConfig',
    fileName: 'nginx.conf',
    language: 'text',
  },
};

export default function ConfigModal({ open, onClose }: ConfigModalProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [source, setSource] = useState<ConfigSource>('xray-core');
  const [configText, setConfigText] = useState('');
  const [loading, setLoading] = useState(false);
  const requestRef = useRef(0);

  useEffect(() => {
    if (!open) return;
    const request = ++requestRef.current;
    const selected = configSources[source];
    setLoading(true);
    setConfigText('');

    HttpUtil.get<unknown>(selected.endpoint)
      .then((msg) => {
        if (request !== requestRef.current || !msg?.success) return;
        if (source === 'xray-core') {
          setConfigText(msg.obj == null ? '' : JSON.stringify(msg.obj, null, 2));
        } else {
          setConfigText(typeof msg.obj === 'string' ? msg.obj : '');
        }
      })
      .finally(() => {
        if (request === requestRef.current) setLoading(false);
      });
  }, [open, source]);

  async function copyConfig() {
    const ok = await ClipboardManager.copyText(configText);
    if (ok) messageApi.success(t('copied'));
  }

  function downloadConfig() {
    FileManager.downloadTextFile(configText, configSources[source].fileName);
  }

  const fileName = configSources[source].fileName;

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={t('pages.index.config')}
        width={isMobile ? '100%' : 900}
        style={
          isMobile
            ? { top: 20, maxWidth: 'calc(100vw - 16px)' }
            : { top: 20 }
        }
        onCancel={onClose}
        footer={[
          <Button
            key="download"
            onClick={downloadConfig}
            disabled={loading}
            size={isMobile ? 'small' : 'middle'}
            icon={<DownloadOutlined />}
          >
            {isMobile ? t('download') : fileName}
          </Button>,
          <Button
            key="copy"
            type="primary"
            onClick={copyConfig}
            disabled={loading}
            size={isMobile ? 'small' : 'middle'}
            icon={<CopyOutlined />}
          >
            {t('copy')}
          </Button>,
        ]}
      >
        <Select<ConfigSource>
          value={source}
          onChange={setSource}
          style={{ width: 180, marginBottom: 12 }}
          aria-label={t('pages.index.config')}
          options={[
            { value: 'xray-core', label: 'Xray Core' },
            { value: 'nginx', label: 'Nginx' },
          ]}
        />
        <Spin spinning={loading}>
          <JsonEditor
            value={configText}
            onChange={setConfigText}
            language={configSources[source].language}
            minHeight={isMobile ? '300px' : 'calc(100vh - 260px)'}
            maxHeight={isMobile ? '70vh' : 'calc(100vh - 260px)'}
            readOnly
          />
        </Spin>
      </Modal>
    </>
  );
}
