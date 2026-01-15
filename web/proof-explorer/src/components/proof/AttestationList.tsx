// Attestation List - Display validator attestations with quorum status
import React from 'react';
import { Card, Table, Typography, Space, Tag, Progress, Tooltip, Avatar } from 'antd';
import { RiUserLine, RiCheckboxCircleFill, RiCloseCircleFill, RiShieldCheckLine } from 'react-icons/ri';
import { ProofAttestation } from '../../types/proof';
import { HashDisplay } from '../common/HashDisplay';

const { Text, Title } = Typography;

interface AttestationListProps {
  attestations?: ProofAttestation[];
  loading?: boolean;
  requiredQuorum?: number; // e.g., 2/3 + 1 = 67% minimum
}

export function AttestationList({
  attestations,
  loading,
  requiredQuorum = 67,
}: AttestationListProps) {
  const validCount = attestations?.filter((a) => a.is_valid).length || 0;
  const totalCount = attestations?.length || 0;
  const quorumPercent = totalCount > 0 ? Math.round((validCount / totalCount) * 100) : 0;
  const quorumMet = quorumPercent >= requiredQuorum;

  const columns = [
    {
      title: 'Validator',
      dataIndex: 'validator_id',
      key: 'validator_id',
      render: (id: string, record: ProofAttestation) => (
        <Space>
          <Avatar size="small" icon={<RiUserLine />} />
          <div>
            <Text strong style={{ fontSize: 12 }}>
              {id.slice(0, 16)}...
            </Text>
            <br />
            {record.validator_pubkey && (
              <Text type="secondary" style={{ fontSize: 10 }}>
                {record.validator_pubkey.slice(0, 12)}...
              </Text>
            )}
          </div>
        </Space>
      ),
    },
    {
      title: 'Signature',
      dataIndex: 'signature',
      key: 'signature',
      render: (sig: string) => (
        <Tooltip title={sig}>
          <Text code style={{ fontSize: 10 }}>
            {sig.slice(0, 24)}...
          </Text>
        </Tooltip>
      ),
    },
    {
      title: 'Attested At',
      dataIndex: 'attested_at',
      key: 'attested_at',
      render: (date: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {new Date(date).toLocaleString()}
        </Text>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'is_valid',
      key: 'is_valid',
      width: 100,
      render: (valid: boolean) =>
        valid ? (
          <Tag color="green" icon={<RiCheckboxCircleFill />}>
            Valid
          </Tag>
        ) : (
          <Tag color="red" icon={<RiCloseCircleFill />}>
            Invalid
          </Tag>
        ),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <RiShieldCheckLine />
          <Title level={5} style={{ margin: 0 }}>
            Validator Attestations
          </Title>
          {quorumMet ? (
            <Tag color="green" icon={<RiCheckboxCircleFill />}>
              Quorum Met
            </Tag>
          ) : totalCount > 0 ? (
            <Tag color="orange">Collecting...</Tag>
          ) : (
            <Tag>No Attestations</Tag>
          )}
        </Space>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {/* Quorum Progress */}
        <div
          style={{
            padding: 16,
            backgroundColor: 'rgba(230, 126, 34, 0.05)',
            borderRadius: 8,
            border: '1px solid rgba(230, 126, 34, 0.15)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
            <Text>Quorum Progress</Text>
            <Text strong>
              {validCount}/{totalCount} validators ({quorumPercent}%)
            </Text>
          </div>
          <Progress
            percent={quorumPercent}
            success={{ percent: requiredQuorum, strokeColor: '#10b981' }}
            strokeColor={quorumMet ? '#10b981' : '#E67E22'}
            trailColor="rgba(230, 126, 34, 0.1)"
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            Required: {requiredQuorum}% (2/3 + 1)
          </Text>
        </div>

        {/* Attestations Table */}
        <Table
          dataSource={attestations}
          columns={columns}
          rowKey="attestation_id"
          loading={loading}
          pagination={false}
          size="small"
          locale={{ emptyText: 'No attestations yet' }}
        />
      </Space>
    </Card>
  );
}

export default AttestationList;
