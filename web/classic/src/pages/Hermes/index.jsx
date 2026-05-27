/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Empty,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconExternalOpen, IconRefresh } from '@douyinfe/semi-icons';
import { API, copy, showError, showSuccess } from '../../helpers';

const { Text, Title } = Typography;

function StatusTag({ value }) {
  const normalized = value || 'unknown';
  const healthy = ['active', 'ready', 'paired', 'completed'].includes(
    normalized,
  );
  return (
    <Tag color={healthy ? 'green' : 'grey'} shape='circle'>
      {normalized}
    </Tag>
  );
}

function DetailRow({ label, value }) {
  return (
    <div className='flex flex-col gap-1 rounded-lg border border-solid border-[var(--semi-color-border)] px-3 py-2 sm:flex-row sm:items-center sm:justify-between'>
      <Text type='tertiary'>{label}</Text>
      <Text strong className='break-all'>
        {value || '-'}
      </Text>
    </div>
  );
}

const Hermes = () => {
  const { t } = useTranslation();
  const [tenant, setTenant] = useState(null);
  const [pairing, setPairing] = useState(null);
  const [tenantUsers, setTenantUsers] = useState(null);
  const [loading, setLoading] = useState(true);
  const [deployingUserID, setDeployingUserID] = useState(null);
  const [pairingUserID, setPairingUserID] = useState(null);

  const reload = async () => {
    setLoading(true);
    try {
      const [tenantRes, pairingRes, adminListRes] = await Promise.all([
        API.get('/api/hermes/tenant/self'),
        API.get('/api/hermes/tenant/pairing/latest'),
        API.get('/api/hermes/tenants', {
          params: { p: 1, page_size: 100 },
          skipErrorHandler: true,
        }).catch(() => null),
      ]);

      setTenant(tenantRes.data?.data?.tenant || null);
      setPairing(pairingRes.data?.data?.session || null);
      setTenantUsers(adminListRes?.data?.data?.items || null);
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  const deployForUser = async (userID) => {
    setDeployingUserID(userID);
    try {
      const res = await API.post(`/api/hermes/tenants/user/${userID}/deploy`);
      if (res.data?.success) {
        showSuccess(t('Hermes Agent 部署已提交'));
        await reload();
      } else {
        showError(res.data?.message || t('Hermes Agent 部署失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setDeployingUserID(null);
    }
  };

  const pairForUser = async (userID) => {
    setPairingUserID(userID);
    try {
      const res = await API.post(
        `/api/hermes/tenants/user/${userID}/pairing/start`,
      );
      if (res.data?.success) {
        showSuccess(t('飞书配对 URL 已生成'));
        await reload();
      } else {
        showError(res.data?.message || t('飞书配对 URL 生成失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setPairingUserID(null);
    }
  };

  const pairCurrentUser = async () => {
    setPairingUserID(tenant?.user_id || 0);
    try {
      const res = await API.post('/api/hermes/tenant/pairing/start');
      if (res.data?.success) {
        showSuccess(t('飞书配对 URL 已生成'));
        await reload();
      } else {
        showError(res.data?.message || t('飞书配对 URL 生成失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setPairingUserID(null);
    }
  };

  useEffect(() => {
    reload();
  }, []);

  const adminDashboardUrl = (userID) =>
    `/api/hermes/tenants/user/${userID}/dashboard/`;
  const shellCommand = (targetTenant) => {
    if (
      !targetTenant?.zeabur_service_id ||
      !targetTenant?.zeabur_environment_id
    ) {
      return '';
    }
    return `zeabur service exec --id ${targetTenant.zeabur_service_id} --env-id ${targetTenant.zeabur_environment_id} -- sh`;
  };
  const copyShellCommand = async (targetTenant) => {
    const command = shellCommand(targetTenant);
    if (!command) {
      showError(t('缺少 Zeabur Service ID 或 Environment ID'));
      return;
    }
    if (await copy(command)) {
      showSuccess(t('Shell 命令已复制'));
    } else {
      showError(t('复制失败'));
    }
  };

  const columns = useMemo(
    () => [
      {
        title: t('用户'),
        dataIndex: 'username',
        render: (_, record) => (
          <div>
            <Text strong>{record.display_name || record.username}</Text>
            <br />
            <Text type='tertiary' size='small'>
              #{record.user_id} {record.email || record.username}
            </Text>
          </div>
        ),
      },
      {
        title: t('租户'),
        dataIndex: 'tenant',
        render: (_, record) => (
          <Space vertical align='start'>
            <StatusTag value={record.tenant?.status} />
            <Text type='tertiary' size='small'>
              {record.tenant?.tenant_id || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('运行时'),
        dataIndex: 'runtime',
        render: (_, record) => (
          <div className='max-w-[260px]'>
            <Text className='break-all'>
              {record.tenant?.service_name || '-'}
            </Text>
            <br />
            <Text type='tertiary' size='small' className='break-all'>
              {record.tenant?.zeabur_service_id || ''}
            </Text>
          </div>
        ),
      },
      {
        title: t('飞书配对'),
        dataIndex: 'pairing',
        render: (_, record) => (
          <Space vertical align='start'>
            <StatusTag value={record.latest_pairing?.status} />
            <Text type='tertiary' size='small'>
              {record.latest_pairing?.id ? `#${record.latest_pairing.id}` : '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('入口'),
        dataIndex: 'public_url',
        render: (_, record) =>
          record.tenant?.dashboard_url ? (
            <Button
              theme='outline'
              icon={<IconExternalOpen />}
              onClick={() =>
                window.open(adminDashboardUrl(record.user_id), '_blank')
              }
            >
              {t('打开')}
            </Button>
          ) : (
            <Text type='tertiary'>-</Text>
          ),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        render: (_, record) => (
          <Space>
            <Button
              type='primary'
              theme='outline'
              loading={deployingUserID === record.user_id}
              disabled={deployingUserID !== null}
              onClick={() => deployForUser(record.user_id)}
            >
              {record.tenant ? t('同步部署') : t('部署')}
            </Button>
            {record.tenant?.dashboard_url ? (
              <Button
                theme='outline'
                loading={pairingUserID === record.user_id}
                disabled={pairingUserID !== null}
                onClick={() => pairForUser(record.user_id)}
              >
                {t('飞书配对')}
              </Button>
            ) : null}
            {shellCommand(record.tenant) ? (
              <Button
                theme='outline'
                onClick={() => copyShellCommand(record.tenant)}
              >
                {t('Shell')}
              </Button>
            ) : null}
          </Space>
        ),
      },
    ],
    [deployingUserID, pairingUserID, t],
  );

  const dashboardUrl = '/api/hermes/tenant/dashboard/';
  const pairingUrl = pairing?.pairing_url;
  const canOpenDashboard = Boolean(tenant?.dashboard_url);

  return (
    <div className='mt-[60px] px-2'>
      <div className='mx-auto flex max-w-6xl flex-col gap-4'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <Title heading={3} style={{ margin: 0 }}>
              {t('Hermes Agent')}
            </Title>
            <Text type='tertiary'>
              {t('在 NewAPI 控制台内管理 Hermes 租户交付和飞书配对状态。')}
            </Text>
          </div>
          <Button icon={<IconRefresh />} loading={loading} onClick={reload}>
            {t('刷新')}
          </Button>
        </div>

        {tenantUsers ? (
          <Card>
            <div className='mb-4'>
              <Title heading={5} style={{ margin: 0 }}>
                {t('Hermes Tenant Control Plane')}
              </Title>
              <Text type='tertiary'>
                {t('超级管理员可以查看所有 NewAPI 用户及其 Hermes 租户状态。')}
              </Text>
            </div>
            <Table
              rowKey='user_id'
              columns={columns}
              dataSource={tenantUsers}
              loading={loading}
              pagination={false}
              empty={<Empty description={t('暂无用户')} />}
              scroll={{ x: true }}
            />
          </Card>
        ) : null}

        <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]'>
          <Card>
            <div className='mb-4 flex items-start justify-between gap-3'>
              <div>
                <Title heading={5} style={{ margin: 0 }}>
                  {t('当前账户租户')}
                </Title>
                <Text type='tertiary'>
                  {t('一个 NewAPI 账户对应一个隔离 Hermes 租户。')}
                </Text>
              </div>
              {loading ? (
                <Spin size='small' />
              ) : (
                <StatusTag value={tenant?.status} />
              )}
            </div>
            <Space vertical align='stretch' style={{ width: '100%' }}>
              <DetailRow label={t('Tenant ID')} value={tenant?.tenant_id} />
              <DetailRow
                label={t('Service Name')}
                value={tenant?.service_name}
              />
              <DetailRow
                label={t('Dashboard')}
                value={tenant?.dashboard_url ? t('NewAPI 中转') : '-'}
              />
              <DetailRow
                label={t('Last Health Status')}
                value={tenant?.last_health_status}
              />
              <Button
                type='primary'
                icon={<IconExternalOpen />}
                disabled={!canOpenDashboard}
                onClick={() => window.open(dashboardUrl, '_blank')}
              >
                {t('打开 Hermes Dashboard')}
              </Button>
              <Button
                theme='outline'
                disabled={!shellCommand(tenant)}
                onClick={() => copyShellCommand(tenant)}
              >
                {t('复制 Shell 命令')}
              </Button>
            </Space>
          </Card>

          <Card>
            <div className='mb-4 flex items-start justify-between gap-3'>
              <div>
                <Title heading={5} style={{ margin: 0 }}>
                  {t('飞书配对')}
                </Title>
                <Text type='tertiary'>
                  {t('配对 URL 由 Hermes 容器命令行生成后回写。')}
                </Text>
              </div>
              {loading ? (
                <Spin size='small' />
              ) : (
                <StatusTag value={pairing?.status} />
              )}
            </div>
            <Space vertical align='stretch' style={{ width: '100%' }}>
              <DetailRow label={t('Session ID')} value={pairing?.id} />
              <DetailRow label={t('Expires At')} value={pairing?.expires_at} />
              {pairingUrl ? (
                <>
                  <Button
                    theme='outline'
                    icon={<IconExternalOpen />}
                    onClick={() => window.open(pairingUrl, '_blank')}
                  >
                    {t('打开配对 URL')}
                  </Button>
                  <Button
                    theme='outline'
                    loading={pairingUserID !== null}
                    disabled={!canOpenDashboard || pairingUserID !== null}
                    onClick={pairCurrentUser}
                  >
                    {t('重新生成配对 URL')}
                  </Button>
                </>
              ) : (
                <div className='rounded-lg border border-dashed border-[var(--semi-color-border)] p-4'>
                  <Text type='tertiary'>{t('暂无有效配对 URL。')}</Text>
                  <div className='mt-3'>
                    <Button
                      theme='outline'
                      loading={pairingUserID !== null}
                      disabled={!canOpenDashboard || pairingUserID !== null}
                      onClick={pairCurrentUser}
                    >
                      {t('生成配对 URL')}
                    </Button>
                  </div>
                </div>
              )}
            </Space>
          </Card>
        </div>
      </div>
    </div>
  );
};

export default Hermes;
