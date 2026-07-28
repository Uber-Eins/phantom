import { lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Layout,
  message,
  Result,
  Row,
  Space,
  Spin,
  Statistic,
  Tag,
  Tooltip,
} from 'antd';
import {
  BarsOutlined,
  ControlOutlined,
  CloudServerOutlined,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  ArrowUpOutlined,
  ArrowDownOutlined,
  AreaChartOutlined,
  GlobalOutlined,
  SwapOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  ThunderboltOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  ForkOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter, TimeFormatter } from '@/utils';
import { formatPanelVersion } from '@/lib/panel-version';
import { activateOnKey } from '@/utils/a11y';
import { useTheme } from '@/hooks/useTheme';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppSidebar from '@/layouts/AppSidebar';
import { LazyMount } from '@/components/utility';
import { setMessageInstance } from '@/utils/messageBus';
import StatusCard from './StatusCard';
import XrayStatusCard from './XrayStatusCard';
const LogModal = lazy(() => import('./LogModal'));
const ConfigModal = lazy(() => import('./ConfigModal'));
const BackupModal = lazy(() => import('./BackupModal'));
const SystemHistoryModal = lazy(() => import('./SystemHistoryModal'));
const XrayMetricsModal = lazy(() => import('./XrayMetricsModal'));
const XrayLogModal = lazy(() => import('./XrayLogModal'));
const VersionModal = lazy(() => import('./VersionModal'));
import './IndexPage.css';

export default function IndexPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { status, fetched, fetchError, refresh } = useStatusQuery();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const [accessLogEnable, setAccessLogEnable] = useState(false);

  const [showIp, setShowIp] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [backupOpen, setBackupOpen] = useState(false);
  const [sysHistoryOpen, setSysHistoryOpen] = useState(false);
  const [xrayMetricsOpen, setXrayMetricsOpen] = useState(false);
  const [xrayLogsOpen, setXrayLogsOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [configTextOpen, setConfigTextOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingTip, setLoadingTip] = useState(t('loading'));

  useEffect(() => {
    HttpUtil.post<{ accessLogEnable?: boolean }>(
      '/panel/api/setting/defaultSettings',
    ).then((msg) => {
      if (msg?.success && msg.obj) {
        setAccessLogEnable(!!msg.obj.accessLogEnable);
      }
    });
  }, []);

  const displayVersion = useMemo(() => window.X_UI_CUR_VER || '?', []);

  const setBusy = useCallback(
    ({ busy, tip }: { busy: boolean; tip?: string }) => {
      setLoading(busy);
      if (tip) setLoadingTip(tip);
    },
    [],
  );

  const stopXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopXrayService');
    await refresh();
  }, [refresh]);

  const restartXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartXrayService');
    await refresh();
  }, [refresh]);

  const pageClass = `index-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Spin
              spinning={loading || !fetched}
              delay={200}
              description={loading ? loadingTip : t('loading')}
              size="large"
            >
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={<Button type="primary" onClick={refresh}>{t('refresh')}</Button>}
                />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, 12]}>
                  <Col span={24}>
                    <StatusCard status={status} isMobile={isMobile} />
                  </Col>

                  <Col xs={24} lg={12}>
                    <XrayStatusCard
                      status={status}
                      isMobile={isMobile}
                      accessLogEnable={accessLogEnable}
                      onStopXray={stopXray}
                      onRestartXray={restartXray}
                      onOpenXrayLogs={() => setXrayLogsOpen(true)}
                      onOpenLogs={() => setLogsOpen(true)}
                      onOpenVersionSwitch={() => setVersionOpen(true)}
                    />
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card
                      title={t('menu.link')}
                      hoverable
                      actions={[
                        <Space className="action" key="logs" role="button" tabIndex={0} aria-label={t('pages.index.logs')} onClick={() => setLogsOpen(true)} onKeyDown={activateOnKey(() => setLogsOpen(true))}>
                          <BarsOutlined />
                          {!isMobile && <span>{t('pages.index.logs')}</span>}
                        </Space>,
                        <Space className="action" key="config" role="button" tabIndex={0} aria-label={t('pages.index.config')} onClick={() => setConfigTextOpen(true)} onKeyDown={activateOnKey(() => setConfigTextOpen(true))}>
                          <ControlOutlined />
                          {!isMobile && <span>{t('pages.index.config')}</span>}
                        </Space>,
                        <Space className="action" key="backup" role="button" tabIndex={0} aria-label={t('pages.index.backupTitle')} onClick={() => setBackupOpen(true)} onKeyDown={activateOnKey(() => setBackupOpen(true))}>
                          <CloudServerOutlined />
                          {!isMobile && <span>{t('pages.index.backupTitle')}</span>}
                        </Space>,
                      ]}
                    />
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card
                      title={
                        <Space>
                          <span>Phantom</span>
                          {isMobile && displayVersion && (
                            <Tag color="green">{formatPanelVersion(displayVersion)}</Tag>
                          )}
                        </Space>
                      }
                      hoverable
                      actions={[
                        <span key="panel-version" className="action">
                          {formatPanelVersion(displayVersion)}
                        </span>,
                      ]}
                    />
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card
                      title={t('pages.index.charts')}
                      hoverable
                      actions={[
                        <Space
                          className="action"
                          key="sys-history"
                          role="button"
                          tabIndex={0}
                          aria-label={t('pages.index.systemHistoryTitle')}
                          onClick={() => setSysHistoryOpen(true)}
                          onKeyDown={activateOnKey(() => setSysHistoryOpen(true))}
                        >
                          <AreaChartOutlined />
                          {!isMobile && <span>{t('pages.index.systemHistoryTitle')}</span>}
                        </Space>,
                        <Space
                          className="action"
                          key="xray-metrics"
                          role="button"
                          tabIndex={0}
                          aria-label={t('pages.index.xrayMetricsTitle')}
                          onClick={() => setXrayMetricsOpen(true)}
                          onKeyDown={activateOnKey(() => setXrayMetricsOpen(true))}
                        >
                          <AreaChartOutlined />
                          {!isMobile && <span>{t('pages.index.xrayMetricsTitle')}</span>}
                        </Space>,
                      ]}
                    />
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card title={t('pages.index.operationHours')} hoverable>
                      <Row gutter={isMobile ? [8, 8] : 0}>
                        <Col span={12}>
                          <Statistic
                            title="Xray"
                            value={TimeFormatter.formatSecond(status.appStats.uptime)}
                            prefix={<ThunderboltOutlined />}
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title="OS"
                            value={TimeFormatter.formatSecond(status.uptime)}
                            prefix={<DesktopOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card title={t('usage')} hoverable>
                      <Row gutter={isMobile ? [8, 8] : 0}>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.memory')}
                            value={SizeFormatter.sizeFormat(status.appStats.mem)}
                            prefix={<DatabaseOutlined />}
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.threads')}
                            value={status.appStats.threads}
                            prefix={<ForkOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card title={t('pages.index.overallSpeed')} hoverable>
                      <Row gutter={isMobile ? [8, 8] : 0}>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.upload')}
                            value={SizeFormatter.sizeFormat(status.netIO.up)}
                            prefix={<ArrowUpOutlined />}
                            suffix="/s"
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.download')}
                            value={SizeFormatter.sizeFormat(status.netIO.down)}
                            prefix={<ArrowDownOutlined />}
                            suffix="/s"
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card title={t('pages.index.totalData')} hoverable>
                      <Row gutter={isMobile ? [8, 8] : 0}>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.sent')}
                            value={SizeFormatter.sizeFormat(status.netTraffic.sent)}
                            prefix={<CloudUploadOutlined />}
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title={t('pages.index.received')}
                            value={SizeFormatter.sizeFormat(status.netTraffic.recv)}
                            prefix={<CloudDownloadOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card
                      title={t('pages.index.ipAddresses')}
                      hoverable
                      extra={
                        <Tooltip
                          title={t('pages.index.toggleIpVisibility')}
                          placement={isMobile ? 'topRight' : 'top'}
                        >
                          {showIp ? (
                            <EyeOutlined
                              className="ip-toggle-icon"
                              role="button"
                              tabIndex={0}
                              aria-label={t('pages.index.toggleIpVisibility')}
                              onClick={() => setShowIp(false)}
                              onKeyDown={activateOnKey(() => setShowIp(false))}
                            />
                          ) : (
                            <EyeInvisibleOutlined
                              className="ip-toggle-icon"
                              role="button"
                              tabIndex={0}
                              aria-label={t('pages.index.toggleIpVisibility')}
                              onClick={() => setShowIp(true)}
                              onKeyDown={activateOnKey(() => setShowIp(true))}
                            />
                          )}
                        </Tooltip>
                      }
                    >
                      <Row className={showIp ? 'ip-visible' : 'ip-hidden'} gutter={isMobile ? [8, 8] : 0}>
                        <Col span={isMobile ? 24 : 12}>
                          <Statistic
                            title="IPv4"
                            value={status.publicIP.ipv4}
                            prefix={<GlobalOutlined />}
                          />
                        </Col>
                        <Col span={isMobile ? 24 : 12}>
                          <Statistic
                            title="IPv6"
                            value={status.publicIP.ipv6}
                            prefix={<GlobalOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card title={t('pages.index.connectionCount')} hoverable>
                      <Row gutter={isMobile ? [8, 8] : 0}>
                        <Col span={12}>
                          <Statistic
                            title="TCP"
                            value={status.tcpCount}
                            prefix={<SwapOutlined />}
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title="UDP"
                            value={status.udpCount}
                            prefix={<SwapOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <LazyMount when={logsOpen}>
          <LogModal open={logsOpen} onClose={() => setLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={backupOpen}>
          <BackupModal
            open={backupOpen}
            onClose={() => setBackupOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>
        <LazyMount when={sysHistoryOpen}>
          <SystemHistoryModal
            open={sysHistoryOpen}
            status={status}
            onClose={() => setSysHistoryOpen(false)}
          />
        </LazyMount>
        <LazyMount when={xrayMetricsOpen}>
          <XrayMetricsModal open={xrayMetricsOpen} onClose={() => setXrayMetricsOpen(false)} />
        </LazyMount>
        <LazyMount when={xrayLogsOpen}>
          <XrayLogModal open={xrayLogsOpen} onClose={() => setXrayLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={versionOpen}>
          <VersionModal
            open={versionOpen}
            status={status}
            onClose={() => setVersionOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>

        <LazyMount when={configTextOpen}>
          <ConfigModal
            open={configTextOpen}
            onClose={() => setConfigTextOpen(false)}
          />
        </LazyMount>
      </Layout>
    </ConfigProvider>
  );
}
