// Account Timeline Page - List proofs for an account
import React, { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Card, Table, Typography, Space, Tag, Breadcrumb, Select, DatePicker, Button, Row, Col } from 'antd';
import { RiHomeLine, RiDownloadLine, RiFilterLine } from 'react-icons/ri';

import { StatusBadge, HashDisplay } from '../components/common';
import { useAsync } from '../hooks';
import proofApi from '../api/proofApi';
import { ProofStatus, GovernanceLevel } from '../types/proof';

const { Title, Text } = Typography;
const { RangePicker } = DatePicker;

function AccountTimeline() {
  const { accountUrl } = useParams();
  const decodedUrl = decodeURIComponent(accountUrl || '');

  // Filters
  const [status, setStatus] = useState<ProofStatus | undefined>();
  const [govLevel, setGovLevel] = useState<GovernanceLevel | undefined>();
  const [page, setPage] = useState(1);
  const pageSize = 20;

  // Fetch proofs
  const proofs = useAsync(
    () =>
      proofApi.getProofsByAccount(decodedUrl, {
        limit: pageSize,
        offset: (page - 1) * pageSize,
        status,
        gov_level: govLevel,
      }),
    [decodedUrl, page, status, govLevel]
  );

  const columns = [
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
        level ? (
          <Tag color={{ G0: '#E67E22', G1: '#a855f7', G2: '#06b6d4' }[level]}>{level}</Tag>
        ) : (
          '-'
        ),
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
    {
      title: 'Anchor',
      dataIndex: 'anchor_chain',
      key: 'anchor_chain',
      render: (chain: string, record: any) =>
        chain ? (
          <Space size="small">
            <Tag>{chain}</Tag>
            {record.anchor_block_number && (
              <Text type="secondary" style={{ fontSize: 11 }}>
                #{record.anchor_block_number}
              </Text>
            )}
          </Space>
        ) : (
          '-'
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
        <Breadcrumb.Item>Account</Breadcrumb.Item>
        <Breadcrumb.Item>{decodedUrl}</Breadcrumb.Item>
      </Breadcrumb>

      {/* Header */}
      <Card style={{ marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>
          Account Proof Timeline
        </Title>
        <Text type="secondary" code>
          {decodedUrl}
        </Text>
      </Card>

      {/* Filters */}
      <Card style={{ marginBottom: 16 }} size="small">
        <Row gutter={16} align="middle">
          <Col>
            <RiFilterLine /> Filters:
          </Col>
          <Col>
            <Select
              placeholder="Status"
              allowClear
              style={{ width: 150 }}
              value={status}
              onChange={setStatus}
            >
              <Select.Option value="pending">Pending</Select.Option>
              <Select.Option value="batched">Batched</Select.Option>
              <Select.Option value="anchored">Anchored</Select.Option>
              <Select.Option value="attested">Attested</Select.Option>
              <Select.Option value="verified">Verified</Select.Option>
              <Select.Option value="failed">Failed</Select.Option>
            </Select>
          </Col>
          <Col>
            <Select
              placeholder="Governance Level"
              allowClear
              style={{ width: 150 }}
              value={govLevel}
              onChange={setGovLevel}
            >
              <Select.Option value="G0">G0 - Execution</Select.Option>
              <Select.Option value="G1">G1 - Governance</Select.Option>
              <Select.Option value="G2">G2 - Outcome</Select.Option>
            </Select>
          </Col>
          <Col flex="auto" />
          <Col>
            <Button icon={<RiDownloadLine />}>Export</Button>
          </Col>
        </Row>
      </Card>

      {/* Proofs Table */}
      <Card>
        <Table
          dataSource={proofs.data?.proofs}
          columns={columns}
          rowKey="proof_id"
          loading={proofs.loading}
          pagination={{
            current: page,
            pageSize,
            total: proofs.data?.total || 0,
            onChange: setPage,
            showSizeChanger: false,
            showTotal: (total) => `${total} proofs`,
          }}
          size="small"
        />
      </Card>
    </div>
  );
}

export default AccountTimeline;
