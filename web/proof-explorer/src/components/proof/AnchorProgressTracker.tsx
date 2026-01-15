// Anchor Progress Tracker - Track anchor confirmations
import React from 'react';
import { Card, Progress, Typography, Space, Tag, Statistic, Row, Col } from 'antd';
import { RiAnchorLine, RiCheckboxCircleFill, RiTimeLine, RiExternalLinkLine } from 'react-icons/ri';
import { AnchorReference } from '../../types/proof';

const { Text, Title } = Typography;

interface AnchorProgressTrackerProps {
  anchor?: AnchorReference;
  loading?: boolean;
  requiredConfirmations?: number;
}

const chainConfig: Record<string, { name: string; color: string; explorerUrl: string }> = {
  ethereum: {
    name: 'Ethereum',
    color: '#627eea',
    explorerUrl: 'https://etherscan.io/tx/',
  },
  sepolia: {
    name: 'Sepolia (Testnet)',
    color: '#a855f7', // Certen purple
    explorerUrl: 'https://sepolia.etherscan.io/tx/',
  },
  bitcoin: {
    name: 'Bitcoin',
    color: '#E67E22', // Certen orange
    explorerUrl: 'https://blockchair.com/bitcoin/transaction/',
  },
};

export function AnchorProgressTracker({
  anchor,
  loading,
  requiredConfirmations = 12,
}: AnchorProgressTrackerProps) {
  if (loading) {
    return <Card loading={true}><div style={{ height: 150 }} /></Card>;
  }

  if (!anchor) {
    return (
      <Card title="Anchor Reference">
        <Text type="secondary">No anchor reference available</Text>
      </Card>
    );
  }

  const chain = chainConfig[anchor.target_chain?.toLowerCase()] || {
    name: anchor.target_chain,
    color: '#888',
    explorerUrl: '#',
  };

  const confirmPercent = Math.min(
    100,
    Math.round((anchor.confirmations / requiredConfirmations) * 100)
  );
  const isConfirmed = anchor.confirmations >= requiredConfirmations;

  return (
    <Card
      title={
        <Space>
          <RiAnchorLine />
          <Title level={5} style={{ margin: 0 }}>
            Anchor Reference
          </Title>
          <Tag color={chain.color}>{chain.name}</Tag>
          {isConfirmed && (
            <Tag color="green" icon={<RiCheckboxCircleFill />}>
              Confirmed
            </Tag>
          )}
        </Space>
      }
    >
      <Row gutter={[24, 16]}>
        <Col xs={24} md={12}>
          <Space direction="vertical" style={{ width: '100%' }}>
            {/* Confirmation Progress */}
            <div>
              <Text type="secondary">Confirmation Progress</Text>
              <Progress
                percent={confirmPercent}
                status={isConfirmed ? 'success' : 'active'}
                format={() => `${anchor.confirmations}/${requiredConfirmations}`}
                strokeColor={isConfirmed ? '#10b981' : '#E67E22'}
              />
            </div>

            {/* Block Number */}
            <Statistic
              title="Anchor Block"
              value={anchor.anchor_block_number}
              prefix={<RiTimeLine />}
            />
          </Space>
        </Col>

        <Col xs={24} md={12}>
          <Space direction="vertical" style={{ width: '100%' }}>
            {/* Transaction Hash */}
            <div>
              <Text type="secondary">Anchor Transaction</Text>
              <br />
              <Space>
                <Text code style={{ fontSize: 12 }}>
                  {anchor.anchor_tx_hash?.slice(0, 20)}...{anchor.anchor_tx_hash?.slice(-8)}
                </Text>
                <a
                  href={`${chain.explorerUrl}${anchor.anchor_tx_hash}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <RiExternalLinkLine /> View
                </a>
              </Space>
            </div>

            {/* Status */}
            <div>
              <Text type="secondary">Status</Text>
              <br />
              {isConfirmed ? (
                <Tag color="green" style={{ marginTop: 4 }}>
                  Finalized ({anchor.confirmations} confirmations)
                </Tag>
              ) : (
                <Tag color="gold" style={{ marginTop: 4 }}>
                  Pending ({requiredConfirmations - anchor.confirmations} more needed)
                </Tag>
              )}
            </div>
          </Space>
        </Col>
      </Row>
    </Card>
  );
}

export default AnchorProgressTracker;
