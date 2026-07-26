import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Modal, Radio, Spin, Tag } from 'antd';

import type { Status } from '@/models/status';
import { HttpUtil } from '@/utils';
import './VersionModal.css';

interface BusyEvent {
  busy: boolean;
  tip?: string;
}

interface VersionModalProps {
  open: boolean;
  status: Status;
  onClose: () => void;
  onBusy: (event: BusyEvent) => void;
}

export default function VersionModal({ open, status, onClose, onBusy }: VersionModalProps) {
  const { t } = useTranslation();
  const [modal, modalContextHolder] = Modal.useModal();
  const [versions, setVersions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchVersions = useCallback(async () => {
    setLoading(true);
    try {
      const response = await HttpUtil.get<string[]>('/panel/api/server/getXrayVersion');
      if (response?.success) setVersions(response.obj || []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (open) void fetchVersions();
  }, [open, fetchVersions]);

  function switchXrayVersion(version: string) {
    modal.confirm({
      title: t('pages.index.xraySwitchVersionDialog'),
      content: t('pages.index.xraySwitchVersionDialogDesc').replace('#version#', version),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: async () => {
        onClose();
        onBusy({ busy: true, tip: t('pages.index.dontRefresh') });
        try {
          await HttpUtil.post(`/panel/api/server/installXray/${encodeURIComponent(version)}`);
        } finally {
          onBusy({ busy: false });
        }
      },
    });
  }

  return (
    <Modal open={open} title={t('pages.index.xrayUpdates')} footer={null} onCancel={onClose}>
      {modalContextHolder}
      <Spin spinning={loading}>
        <Alert
          type="warning"
          className="mb-12"
          title={t('pages.index.xraySwitchClickDesk')}
          showIcon
        />
        <div className="version-list">
          {versions.map((version, index) => (
            <div key={version} className="version-list-item">
              <Tag color={index % 2 === 0 ? 'purple' : 'green'}>{version}</Tag>
              <Radio
                checked={version === `v${status.xray.version}`}
                onClick={() => switchXrayVersion(version)}
              />
            </div>
          ))}
        </div>
      </Spin>
    </Modal>
  );
}
