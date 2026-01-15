// Dashboard Page - Main landing page with search and stats
import React from 'react';
import { Row, Col, Card, Statistic, Table, Typography, Space, Tag, Spin, Alert } from 'antd';
import {
  RiFileListLine,
  RiCheckboxCircleFill,
  RiTimeLine,
  RiUserLine,
  RiAnchorLine,
} from 'react-icons/ri';
import { Link } from 'react-router-dom';

import { SearchForm } from '../components/common';
import { useAsync } from '../hooks';
import proofApi from '../api/proofApi';
import { ProofStatistics, SystemHealth, ProofArtifact } from '../types/proof';

const { Title, Text } = Typography;

function Dashboard() {
  const stats = useAsync<ProofStatistics>(() => proofApi.getProofStats(), []);
  const health = useAsync<SystemHealth>(() => proofApi.getSystemHealth(), []);
  const recentProofs = useAsync(() => proofApi.queryProofs({ limit: 10 }), []);

  const columns = [
    {
      title: 'Proof ID',
      dataIndex: 'proof_id',
      key: 'proof_id',
      render: (id: string) => (
        <Link to={`/proof/${id}`}>
          <Text code style={{ fontSize: 11 }}>
            {id.slice(0, 8)}...
          </Text>
        </Link>
      ),
    },
    {
      title: 'Account',
      dataIndex: 'account_url',
      key: 'account_url',
      render: (url: string) => (
        <Link to={`/account/${encodeURIComponent(url)}`}>
          <Text style={{ fontSize: 11 }}>
            {url.length > 30 ? `${url.slice(0, 30)}...` : url}
          </Text>
        </Link>
      ),
    },
    {
      title: 'Type',
      dataIndex: 'proof_type',
      key: 'proof_type',
      render: (type: string) => <Tag>{type}</Tag>,
    },
    {
      title: 'Gov Level',
      dataIndex: 'gov_level',
      key: 'gov_level',
      render: (level: string) =>
        level ? <Tag color="purple">{level}</Tag> : <Text type="secondary">-</Text>,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const colors: Record<string, string> = {
          pending: 'gold',
          batched: 'blue',
          anchored: 'cyan',
          attested: 'purple',
          verified: 'green',
          failed: 'red',
        };
        return <Tag color={colors[status]}>{status}</Tag>;
      },
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <Text type="secondary" style={{ fontSize: 11 }}>
          {new Date(date).toLocaleString()}
        </Text>
      ),
    },
  ];

  return (
    <div>
      {/* Search Section */}
      <SearchForm />

      {/* System Health Alert */}
      {health.data && health.data.status !== 'healthy' && (
        <Alert
          type={health.data.status === 'degraded' ? 'warning' : 'error'}
          message={`System Status: ${health.data.status}`}
          description="Some components may not be functioning properly"
          showIcon
          style={{ marginBottom: 24 }}
        />
      )}

      {/* Statistics Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Total Proofs"
              value={stats.data?.total_proofs || 0}
              prefix={<RiFileListLine />}
              loading={stats.loading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Verified (24h)"
              value={stats.data?.time_windows.last_24h.proofs_verified || 0}
              prefix={<RiCheckboxCircleFill style={{ color: '#10b981' }} />}
              loading={stats.loading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Active Validators"
              value={health.data?.components.attestation_service.active_validators || 0}
              prefix={<RiUserLine />}
              loading={health.loading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Quorum Rate"
              value={stats.data?.attestation_stats.quorum_rate || 0}
              suffix="%"
              prefix={<RiAnchorLine />}
              loading={stats.loading}
            />
          </Card>
        </Col>
      </Row>

      {/* Stats by Type */}
      {stats.data && (
        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col xs={24} md={8}>
            <Card title="Proofs by Status" size="small">
              {Object.entries(stats.data.proofs_by_status).map(([status, count]) => (
                <div
                  key={status}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    marginBottom: 8,
                  }}
                >
                  <Tag
                    color={
                      {
                        pending: 'gold',
                        batched: 'blue',
                        anchored: 'cyan',
                        attested: 'purple',
                        verified: 'green',
                        failed: 'red',
                      }[status] || 'default'
                    }
                  >
                    {status}
                  </Tag>
                  <Text strong>{count.toLocaleString()}</Text>
                </div>
              ))}
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="Proofs by Type" size="small">
              {Object.entries(stats.data.proofs_by_type).map(([type, count]) => (
                <div
                  key={type}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    marginBottom: 8,
                  }}
                >
                  <Text>{type}</Text>
                  <Text strong>{count.toLocaleString()}</Text>
                </div>
              ))}
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="Proofs by Governance Level" size="small">
              {Object.entries(stats.data.proofs_by_gov_level).map(([level, count]) => (
                <div
                  key={level}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    marginBottom: 8,
                  }}
                >
                  <Tag
                    color={{ G0: '#E67E22', G1: '#a855f7', G2: '#06b6d4' }[level] || 'default'}
                  >
                    {level}
                  </Tag>
                  <Text strong>{count.toLocaleString()}</Text>
                </div>
              ))}
            </Card>
          </Col>
        </Row>
      )}

      {/* Recent Proofs */}
      <Card title="Recent Proofs">
        <Table
          dataSource={recentProofs.data?.proofs}
          columns={columns}
          rowKey="proof_id"
          loading={recentProofs.loading}
          pagination={false}
          size="small"
        />
      </Card>
    </div>
  );
}

export default Dashboard;
