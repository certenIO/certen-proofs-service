// Batch Inspector Page - View all proofs in a batch
import React from 'react';
import { useParams, Link } from 'react-router-dom';
import { Card, Table, Typography, Space, Tag, Breadcrumb, Statistic, Row, Col, Progress, Spin } from 'antd';
import { RiHomeLine, RiGroupLine, RiCheckboxCircleFill, RiTimeLine } from 'react-icons/ri';

import { StatusBadge, HashDisplay } from '../components/common';
import { useAsync } from '../hooks';
import proofApi from '../api/proofApi';
import { ProofStatus } from '../types/proof';

const { Title, Text } = Typography;

function BatchInspector() {
  const { batchId } = useParams();

  // Fetch batch proofs
  const batch = useAsync(() => proofApi.getProofsByBatch(batchId || ''), [batchId]);

  if (batch.loading) {
    return (
      <div style={{ textAlign: 'center', padding: 48 }}>
        <Spin size="large" />
        <p>Loading batch...</p>
      </div>
    );
  }

  const proofs = batch.data?.proofs || [];
  const verifiedCount = proofs.filter((p) => p.status === 'verified').length;
  const attestedCount = proofs.filter((p) => p.status === 'attested').length;
  const anchoredCount = proofs.filter((p) => ['anchored', 'attested', 'verified'].includes(p.status)).length;

  const columns = [
    {
      title: 'Position',
      dataIndex: 'batch_position',
      key: 'batch_position',
      width: 80,
      render: (_: any, __: any, index: number) => (
        <Text strong>{index + 1}</Text>
      ),
    },
    {
      title: 'Proof ID',
      dataIndex: 'proof_id',
      key: 'proof_id',
      render: (id: string) => (
        <Link to={`/proof/${id}`}>
          <HashDisplay hash={id} truncateLength={6} />
        </Link>
      ),
    },
    {
      title: 'TX Hash',
      dataIndex: 'accum_tx_hash',
      key: 'accum_tx_hash',
      render: (hash: string) => <HashDisplay hash={hash} truncateLength={8} />,
    },
    {
      title: 'Account',
      dataIndex: 'account_url',
      key: 'account_url',
      render: (url: string) => (
        <Link to={`/account/${encodeURIComponent(url)}`}>
          <Text style={{ fontSize: 11 }}>
            {url.length > 25 ? `${url.slice(0, 25)}...` : url}
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
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (s: ProofStatus) => <StatusBadge status={s} />,
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {new Date(date).toLocaleString()}
        </Text>
      ),
    },
  ];

  return (
    <div>
      {/* Breadcrumb */}
      <Breadcrumb style={{ marginBottom: 16 }}>
        <Breadcrumb.Item>
          <Link to="/">
            <RiHomeLine /> Home
          </Link>
        </Breadcrumb.Item>
        <Breadcrumb.Item>Batch</Breadcrumb.Item>
        <Breadcrumb.Item>{batchId?.slice(0, 8)}...</Breadcrumb.Item>
      </Breadcrumb>

      {/* Header */}
      <Card style={{ marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>
          Batch Inspector
        </Title>
        <Text type="secondary">
          Batch ID: <Text code>{batchId}</Text>
        </Text>
      </Card>

      {/* Batch Statistics */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Total Proofs"
              value={proofs.length}
              prefix={<RiGroupLine />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Anchored"
              value={anchoredCount}
              suffix={`/ ${proofs.length}`}
              prefix={<RiTimeLine />}
            />
            <Progress
              percent={Math.round((anchoredCount / Math.max(proofs.length, 1)) * 100)}
              size="small"
              strokeColor="#06b6d4"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Attested"
              value={attestedCount}
              suffix={`/ ${proofs.length}`}
              prefix={<RiCheckboxCircleFill style={{ color: '#a855f7' }} />}
            />
            <Progress
              percent={Math.round((attestedCount / Math.max(proofs.length, 1)) * 100)}
              size="small"
              strokeColor="#a855f7"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="Verified"
              value={verifiedCount}
              suffix={`/ ${proofs.length}`}
              prefix={<RiCheckboxCircleFill style={{ color: '#10b981' }} />}
            />
            <Progress
              percent={Math.round((verifiedCount / Math.max(proofs.length, 1)) * 100)}
              size="small"
              status={verifiedCount === proofs.length && proofs.length > 0 ? 'success' : 'active'}
            />
          </Card>
        </Col>
      </Row>

      {/* Anchor Information */}
      {proofs.length > 0 && proofs[0].anchor_tx_hash && (
        <Card style={{ marginBottom: 16 }} size="small">
          <Row gutter={24}>
            <Col>
              <Text type="secondary">Anchor Chain:</Text>{' '}
              <Tag>{proofs[0].anchor_chain || 'unknown'}</Tag>
            </Col>
            <Col>
              <Text type="secondary">Anchor TX:</Text>{' '}
              <HashDisplay
                hash={proofs[0].anchor_tx_hash}
                linkTo={
                  proofs[0].anchor_chain === 'ethereum'
                    ? `https://etherscan.io/tx/${proofs[0].anchor_tx_hash}`
                    : undefined
                }
              />
            </Col>
            {proofs[0].anchor_block_number && (
              <Col>
                <Text type="secondary">Block:</Text>{' '}
                <Text strong>#{proofs[0].anchor_block_number}</Text>
              </Col>
            )}
          </Row>
        </Card>
      )}

      {/* Proofs Table */}
      <Card title={`Proofs in Batch (${proofs.length})`}>
        <Table
          dataSource={proofs}
          columns={columns}
          rowKey="proof_id"
          loading={batch.loading}
          pagination={{
            pageSize: 50,
            showSizeChanger: false,
            showTotal: (total) => `${total} proofs in batch`,
          }}
          size="small"
        />
      </Card>
    </div>
  );
}

export default BatchInspector;
