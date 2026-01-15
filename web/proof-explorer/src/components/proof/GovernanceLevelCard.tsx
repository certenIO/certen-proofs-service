// Governance Level Card - Display G0/G1/G2 proof levels
// Per Whitepaper Section on Governance Proofs
import React from 'react';
import { Card, Collapse, Typography, Space, Tag, Descriptions, Progress } from 'antd';
import {
  RiCheckboxCircleFill,
  RiCloseCircleFill,
  RiShieldCheckLine,
  RiLockLine,
  RiFileListLine,
} from 'react-icons/ri';
import { GovernanceProofLevel, GovernanceLevel } from '../../types/proof';

const { Text, Title } = Typography;
const { Panel } = Collapse;

interface GovernanceLevelCardProps {
  levels?: GovernanceProofLevel[];
  loading?: boolean;
}

const levelConfig: Record<
  GovernanceLevel,
  {
    name: string;
    description: string;
    icon: React.ReactNode;
    color: string;
  }
> = {
  G0: {
    name: 'Execution Inclusion',
    description: 'Proves transaction was included in a finalized block',
    icon: <RiFileListLine />,
    color: '#E67E22', // Certen primary orange
  },
  G1: {
    name: 'Governance Correctness',
    description: 'Proves authority was correctly validated (Key Book verification)',
    icon: <RiShieldCheckLine />,
    color: '#a855f7', // Certen purple
  },
  G2: {
    name: 'Outcome Binding',
    description: 'Proves outcome is cryptographically bound to governance decision',
    icon: <RiLockLine />,
    color: '#06b6d4', // Certen cyan
  },
};

function LevelPanel({ level }: { level: GovernanceProofLevel }) {
  const config = levelConfig[level.gov_level];
  const verified = level.verified;

  return (
    <Panel
      key={level.gov_level}
      header={
        <Space>
          <span style={{ color: config.color }}>{config.icon}</span>
          <Text strong>{level.gov_level}: {config.name}</Text>
          {verified ? (
            <Tag color="green" icon={<RiCheckboxCircleFill />}>
              Verified
            </Tag>
          ) : (
            <Tag color="orange" icon={<RiCloseCircleFill />}>
              Pending
            </Tag>
          )}
        </Space>
      }
    >
      <Descriptions size="small" column={2} bordered>
        <Descriptions.Item label="Level Name">
          {level.level_name || config.name}
        </Descriptions.Item>
        <Descriptions.Item label="Verified">
          {verified ? 'Yes' : 'No'}
          {level.verified_at && (
            <Text type="secondary" style={{ marginLeft: 8 }}>
              at {new Date(level.verified_at).toLocaleString()}
            </Text>
          )}
        </Descriptions.Item>

        {/* G0 specific fields */}
        {level.gov_level === 'G0' && (
          <>
            {level.block_height && (
              <Descriptions.Item label="Block Height">
                {level.block_height.toLocaleString()}
              </Descriptions.Item>
            )}
            {level.finality_timestamp && (
              <Descriptions.Item label="Finality Time">
                {new Date(level.finality_timestamp).toLocaleString()}
              </Descriptions.Item>
            )}
            {level.anchor_height && (
              <Descriptions.Item label="Anchor Height">
                {level.anchor_height.toLocaleString()}
              </Descriptions.Item>
            )}
          </>
        )}

        {/* G1 specific fields */}
        {level.gov_level === 'G1' && (
          <>
            {level.authority_url && (
              <Descriptions.Item label="Authority URL" span={2}>
                <Text code>{level.authority_url}</Text>
              </Descriptions.Item>
            )}
            {level.threshold_m !== undefined && level.threshold_n !== undefined && (
              <Descriptions.Item label="Threshold">
                <Space>
                  <Text>{level.threshold_m} of {level.threshold_n}</Text>
                  <Progress
                    percent={Math.round(((level.signature_count || 0) / level.threshold_m) * 100)}
                    size="small"
                    style={{ width: 100 }}
                  />
                </Space>
              </Descriptions.Item>
            )}
            {level.signature_count !== undefined && (
              <Descriptions.Item label="Signatures">
                {level.signature_count}
              </Descriptions.Item>
            )}
          </>
        )}

        {/* G2 specific fields */}
        {level.gov_level === 'G2' && (
          <>
            {level.outcome_type && (
              <Descriptions.Item label="Outcome Type">
                <Tag>{level.outcome_type}</Tag>
              </Descriptions.Item>
            )}
          </>
        )}
      </Descriptions>
    </Panel>
  );
}

export function GovernanceLevelCard({ levels, loading }: GovernanceLevelCardProps) {
  if (loading) {
    return <Card loading={true}><div style={{ height: 200 }} /></Card>;
  }

  const verifiedCount = levels?.filter((l) => l.verified).length || 0;
  const totalLevels = levels?.length || 0;

  // Sort levels by G0, G1, G2
  const sortedLevels = [...(levels || [])].sort((a, b) => {
    const order: Record<GovernanceLevel, number> = { G0: 0, G1: 1, G2: 2 };
    return order[a.gov_level] - order[b.gov_level];
  });

  return (
    <Card
      title={
        <Space>
          <Title level={5} style={{ margin: 0 }}>
            Governance Proof Levels
          </Title>
          <Tag color={verifiedCount === totalLevels && totalLevels > 0 ? 'green' : 'gold'}>
            {verifiedCount}/{totalLevels} Verified
          </Tag>
        </Space>
      }
    >
      {!levels || levels.length === 0 ? (
        <Text type="secondary">No governance levels available</Text>
      ) : (
        <Collapse defaultActiveKey={['G0']} accordion>
          {sortedLevels.map((level) => (
            <LevelPanel key={level.level_id} level={level} />
          ))}
        </Collapse>
      )}
    </Card>
  );
}

export default GovernanceLevelCard;
