// Merkle Tree Visualization - Display merkle proof path
import React from 'react';
import { Card, Typography, Space, Tag, Tree, Tooltip } from 'antd';
import { RiNodeTree, RiCheckboxCircleFill, RiArrowUpLine } from 'react-icons/ri';
import { MerkleInclusionProof } from '../../types/proof';

const { Text, Title } = Typography;

interface MerkleTreeVisualizationProps {
  merkleProof?: MerkleInclusionProof;
  loading?: boolean;
}

export function MerkleTreeVisualization({
  merkleProof,
  loading,
}: MerkleTreeVisualizationProps) {
  if (loading) {
    return <Card loading={true}><div style={{ height: 200 }} /></Card>;
  }

  if (!merkleProof) {
    return (
      <Card title="Merkle Inclusion Proof">
        <Text type="secondary">No merkle proof available</Text>
      </Card>
    );
  }

  const { merkle_root, leaf_hash, leaf_index, merkle_path } = merkleProof;

  // Build tree data for visualization
  const treeData = [
    {
      title: (
        <Space>
          <Tag color="green">Root</Tag>
          <Text code style={{ fontSize: 11 }}>
            {merkle_root?.slice(0, 20)}...
          </Text>
        </Space>
      ),
      key: 'root',
      children: merkle_path?.map((entry, index) => ({
        title: (
          <Space>
            <Tag color={entry.right ? 'blue' : 'purple'}>
              {entry.right ? 'Right' : 'Left'}
            </Tag>
            <Text code style={{ fontSize: 11 }}>
              {entry.hash?.slice(0, 20)}...
            </Text>
          </Space>
        ),
        key: `path-${index}`,
      })),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <RiNodeTree />
          <Title level={5} style={{ margin: 0 }}>
            Merkle Inclusion Proof
          </Title>
          <Tag color="green">
            <RiCheckboxCircleFill /> Valid
          </Tag>
        </Space>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {/* Summary */}
        <div
          style={{
            display: 'flex',
            gap: 24,
            flexWrap: 'wrap',
            padding: 16,
            backgroundColor: 'rgba(230, 126, 34, 0.05)',
            borderRadius: 8,
            border: '1px solid rgba(230, 126, 34, 0.15)',
          }}
        >
          <div>
            <Text type="secondary">Leaf Index</Text>
            <br />
            <Text strong style={{ fontSize: 18 }}>
              {leaf_index}
            </Text>
          </div>
          <div>
            <Text type="secondary">Path Length</Text>
            <br />
            <Text strong style={{ fontSize: 18 }}>
              {merkle_path?.length || 0} nodes
            </Text>
          </div>
          <div>
            <Text type="secondary">Tree Depth</Text>
            <br />
            <Text strong style={{ fontSize: 18 }}>
              {Math.ceil(Math.log2((merkle_path?.length || 1) + 1))} levels
            </Text>
          </div>
        </div>

        {/* Visual Path */}
        <div>
          <Text type="secondary" style={{ marginBottom: 8, display: 'block' }}>
            Proof Path (Leaf to Root):
          </Text>

          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 4,
              padding: 16,
              border: '1px solid rgba(230, 126, 34, 0.15)',
              borderRadius: 8,
              backgroundColor: 'rgba(255, 255, 255, 0.02)',
            }}
          >
            {/* Leaf */}
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                padding: '8px 12px',
                backgroundColor: 'rgba(6, 182, 212, 0.1)',
                borderRadius: 4,
                border: '1px solid rgba(6, 182, 212, 0.3)',
              }}
            >
              <Tag color="cyan">Leaf</Tag>
              <Tooltip title={leaf_hash}>
                <Text code style={{ fontSize: 11 }}>
                  {leaf_hash?.slice(0, 32)}...
                </Text>
              </Tooltip>
            </div>

            {/* Path entries */}
            {merkle_path?.map((entry, index) => (
              <React.Fragment key={index}>
                <div style={{ textAlign: 'center' }}>
                  <RiArrowUpLine style={{ color: '#E67E22' }} />
                </div>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    padding: '8px 12px',
                    backgroundColor: entry.right ? 'rgba(230, 126, 34, 0.1)' : 'rgba(168, 85, 247, 0.1)',
                    borderRadius: 4,
                    border: entry.right ? '1px solid rgba(230, 126, 34, 0.3)' : '1px solid rgba(168, 85, 247, 0.3)',
                  }}
                >
                  <Tag color={entry.right ? 'gold' : 'purple'}>
                    {entry.right ? 'Right Sibling' : 'Left Sibling'}
                  </Tag>
                  <Tooltip title={entry.hash}>
                    <Text code style={{ fontSize: 11 }}>
                      {entry.hash?.slice(0, 32)}...
                    </Text>
                  </Tooltip>
                </div>
              </React.Fragment>
            ))}

            {/* Root */}
            <div style={{ textAlign: 'center' }}>
              <RiArrowUpLine style={{ color: '#10b981' }} />
            </div>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                padding: '8px 12px',
                backgroundColor: 'rgba(16, 185, 129, 0.1)',
                borderRadius: 4,
                border: '2px solid #10b981',
              }}
            >
              <Tag color="green">Merkle Root</Tag>
              <Tooltip title={merkle_root}>
                <Text code style={{ fontSize: 11 }}>
                  {merkle_root?.slice(0, 32)}...
                </Text>
              </Tooltip>
            </div>
          </div>
        </div>
      </Space>
    </Card>
  );
}

export default MerkleTreeVisualization;
