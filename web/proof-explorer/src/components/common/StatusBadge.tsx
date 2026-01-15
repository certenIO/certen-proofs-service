// Status Badge Component
import React from 'react';
import { Tag } from 'antd';
import { ProofStatus, GovernanceLevel } from '../../types/proof';

interface StatusBadgeProps {
  status: ProofStatus;
}

const statusColors: Record<ProofStatus, string> = {
  pending: 'gold',
  batched: 'blue',
  anchored: 'cyan',
  attested: 'purple',
  verified: 'green',
  failed: 'red',
};

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <Tag color={statusColors[status]} style={{ textTransform: 'capitalize' }}>
      {status}
    </Tag>
  );
}

interface GovLevelBadgeProps {
  level: GovernanceLevel;
  verified?: boolean;
}

const govLevelColors: Record<GovernanceLevel, string> = {
  G0: '#E67E22', // Certen primary orange
  G1: '#a855f7', // Certen purple
  G2: '#06b6d4', // Certen cyan
};

const govLevelNames: Record<GovernanceLevel, string> = {
  G0: 'Execution Inclusion',
  G1: 'Governance Correctness',
  G2: 'Outcome Binding',
};

export function GovLevelBadge({ level, verified }: GovLevelBadgeProps) {
  return (
    <Tag color={govLevelColors[level]}>
      {level}: {govLevelNames[level]}
      {verified !== undefined && (
        <span style={{ marginLeft: 4 }}>
          {verified ? '\u2713' : '\u2717'}
        </span>
      )}
    </Tag>
  );
}

export default StatusBadge;
