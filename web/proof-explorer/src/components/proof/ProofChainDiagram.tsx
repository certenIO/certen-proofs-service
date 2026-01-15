// Proof Chain Diagram - Visual representation of L1/L2/L3 proof layers
// Per Whitepaper Section 3.4.1 - Chained Proof Architecture
import React from 'react';
import { Card, Steps, Tooltip, Typography, Space, Tag } from 'antd';
import {
  RiCheckboxCircleFill,
  RiCloseCircleFill,
  RiLoader4Line,
  RiArrowRightLine,
} from 'react-icons/ri';
import { ChainedProof, ProofLayer } from '../../types/proof';

const { Text, Title } = Typography;
const { Step } = Steps;

interface ProofChainDiagramProps {
  chainedProof?: ChainedProof;
  loading?: boolean;
}

interface LayerCardProps {
  layer?: ProofLayer;
  title: string;
  description: string;
  icon: React.ReactNode;
}

function LayerCard({ layer, title, description, icon }: LayerCardProps) {
  const verified = layer?.verified ?? false;
  const hasData = !!layer;

  return (
    <Card
      size="small"
      style={{
        minWidth: 200,
        borderColor: verified ? '#10b981' : hasData ? '#f59e0b' : 'rgba(230, 126, 34, 0.15)',
        backgroundColor: verified ? 'rgba(16, 185, 129, 0.1)' : hasData ? 'rgba(245, 158, 11, 0.1)' : 'rgba(255, 255, 255, 0.02)',
      }}
    >
      <Space direction="vertical" size="small" style={{ width: '100%' }}>
        <Space>
          {icon}
          <Text strong>{title}</Text>
          {verified ? (
            <RiCheckboxCircleFill style={{ color: '#10b981' }} />
          ) : hasData ? (
            <RiLoader4Line style={{ color: '#f59e0b' }} />
          ) : (
            <RiCloseCircleFill style={{ color: '#64748b' }} />
          )}
        </Space>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {description}
        </Text>
        {layer && (
          <>
            <div style={{ marginTop: 8 }}>
              <Text type="secondary" style={{ fontSize: 11 }}>
                Source:
              </Text>
              <br />
              <Text code style={{ fontSize: 10 }}>
                {layer.source_hash?.slice(0, 16)}...
              </Text>
            </div>
            <div>
              <Text type="secondary" style={{ fontSize: 11 }}>
                Target:
              </Text>
              <br />
              <Text code style={{ fontSize: 10 }}>
                {layer.target_hash?.slice(0, 16)}...
              </Text>
            </div>
            {layer.receipt && (
              <Tag color="blue" style={{ marginTop: 4 }}>
                {layer.receipt.entries?.length || 0} receipt entries
              </Tag>
            )}
          </>
        )}
      </Space>
    </Card>
  );
}

export function ProofChainDiagram({ chainedProof, loading }: ProofChainDiagramProps) {
  if (loading) {
    return (
      <Card loading={true}>
        <div style={{ height: 200 }} />
      </Card>
    );
  }

  const layers = [
    {
      layer: chainedProof?.layer1,
      title: 'Layer 1: TX -> BVN',
      description: 'Transaction to Block Validator Network',
      icon: <span style={{ fontSize: 18 }}>1️</span>,
    },
    {
      layer: chainedProof?.layer2,
      title: 'Layer 2: BVN -> DN',
      description: 'BVN to Directory Network',
      icon: <span style={{ fontSize: 18 }}>2️</span>,
    },
    {
      layer: chainedProof?.layer3,
      title: 'Layer 3: DN -> Consensus',
      description: 'DN to Consensus Root',
      icon: <span style={{ fontSize: 18 }}>3️</span>,
    },
  ];

  const verifiedCount = layers.filter((l) => l.layer?.verified).length;

  return (
    <Card
      title={
        <Space>
          <Title level={5} style={{ margin: 0 }}>
            Chained Proof (L1/L2/L3)
          </Title>
          <Tag color={verifiedCount === 3 ? 'green' : verifiedCount > 0 ? 'gold' : 'default'}>
            {verifiedCount}/3 Verified
          </Tag>
        </Space>
      }
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 16,
          overflowX: 'auto',
          padding: '16px 0',
        }}
      >
        {layers.map((layerInfo, index) => (
          <React.Fragment key={index}>
            <LayerCard {...layerInfo} />
            {index < layers.length - 1 && (
              <RiArrowRightLine
                style={{
                  fontSize: 24,
                  color: layerInfo.layer?.verified ? '#10b981' : '#64748b',
                  flexShrink: 0,
                }}
              />
            )}
          </React.Fragment>
        ))}
      </div>

      {/* Legend */}
      <div style={{ marginTop: 16, borderTop: '1px solid rgba(230, 126, 34, 0.15)', paddingTop: 12 }}>
        <Space>
          <Space size="small">
            <RiCheckboxCircleFill style={{ color: '#10b981' }} />
            <Text type="secondary">Verified</Text>
          </Space>
          <Space size="small">
            <RiLoader4Line style={{ color: '#f59e0b' }} />
            <Text type="secondary">Pending</Text>
          </Space>
          <Space size="small">
            <RiCloseCircleFill style={{ color: '#64748b' }} />
            <Text type="secondary">Missing</Text>
          </Space>
        </Space>
      </div>
    </Card>
  );
}

export default ProofChainDiagram;
