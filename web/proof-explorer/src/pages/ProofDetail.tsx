// Proof Detail Page - Complete proof visualization
import React from 'react';
import { useParams, Link } from 'react-router-dom';
import { Row, Col, Card, Typography, Space, Tag, Breadcrumb, Spin, Alert, Tabs } from 'antd';
import { RiHomeLine } from 'react-icons/ri';

import { InfoTable, StatusBadge, HashDisplay, AccountLink } from '../components/common';
import {
  ProofChainDiagram,
  GovernanceLevelCard,
  MerkleTreeVisualization,
  AnchorProgressTracker,
  AttestationList,
  BundleDownloader,
} from '../components/proof';
import { useAsync } from '../hooks';
import proofApi from '../api/proofApi';

const { Title, Text } = Typography;

function ProofDetail() {
  const { proofId, txHash } = useParams();

  // Fetch proof details
  const proof = useAsync(async () => {
    if (proofId) {
      return proofApi.getProofById(proofId);
    } else if (txHash) {
      const artifact = await proofApi.getProofByTxHash(txHash);
      return proofApi.getProofById(artifact.proof_id);
    }
    throw new Error('No proof ID or TX hash provided');
  }, [proofId, txHash]);

  // Fetch custody chain
  const custody = useAsync(
    () => (proof.data ? proofApi.getCustodyChain(proof.data.proof_id) : Promise.resolve(null)),
    [proof.data?.proof_id]
  );

  if (proof.loading) {
    return (
      <div style={{ textAlign: 'center', padding: 48 }}>
        <Spin size="large" />
        <p>Loading proof details...</p>
      </div>
    );
  }

  if (proof.error) {
    return (
      <Alert
        type="error"
        message="Error Loading Proof"
        description={proof.error.message}
        showIcon
      />
    );
  }

  if (!proof.data) {
    return <Alert type="warning" message="Proof not found" showIcon />;
  }

  const p = proof.data;

  const overviewItems = [
    { label: 'Proof ID', value: <HashDisplay hash={p.proof_id} copyable /> },
    { label: 'Status', value: <StatusBadge status={p.status} /> },
    { label: 'Proof Type', value: <Tag>{p.proof_type}</Tag> },
    { label: 'Proof Class', value: <Tag color="blue">{p.proof_class}</Tag> },
    {
      label: 'Governance Level',
      value: p.gov_level ? <Tag color="purple">{p.gov_level}</Tag> : '-',
    },
    { label: 'Created', value: new Date(p.created_at).toLocaleString() },
    {
      label: 'Anchored',
      value: p.anchored_at ? new Date(p.anchored_at).toLocaleString() : 'Pending',
    },
    {
      label: 'Verified',
      value: p.verified_at ? new Date(p.verified_at).toLocaleString() : 'Pending',
    },
  ];

  const txItems = [
    { label: 'Transaction Hash', value: <HashDisplay hash={p.accum_tx_hash} copyable />, span: 2 },
    { label: 'Account URL', value: <AccountLink url={p.account_url} /> },
    { label: 'Batch ID', value: p.batch_id ? <HashDisplay hash={p.batch_id} /> : '-' },
    {
      label: 'Anchor Chain',
      value: p.anchor_chain ? <Tag>{p.anchor_chain}</Tag> : '-',
    },
    {
      label: 'Anchor TX',
      value: p.anchor_tx_hash ? (
        <HashDisplay
          hash={p.anchor_tx_hash}
          linkTo={
            p.anchor_chain === 'ethereum'
              ? `https://etherscan.io/tx/${p.anchor_tx_hash}`
              : undefined
          }
        />
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
        <Breadcrumb.Item>Proof</Breadcrumb.Item>
        <Breadcrumb.Item>{p.proof_id.slice(0, 8)}...</Breadcrumb.Item>
      </Breadcrumb>

      {/* Header */}
      <Card style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={4} style={{ margin: 0 }}>
              Proof Details
            </Title>
            <Space>
              <StatusBadge status={p.status} />
              {p.gov_level && <Tag color="purple">{p.gov_level}</Tag>}
            </Space>
          </div>
          <Text type="secondary">
            Accumulate TX: <Text code>{p.accum_tx_hash}</Text>
          </Text>
        </Space>
      </Card>

      {/* Main Content Tabs */}
      <Tabs
        defaultActiveKey="overview"
        items={[
          {
            key: 'overview',
            label: 'Overview',
            children: (
              <Row gutter={[16, 16]}>
                <Col xs={24} lg={12}>
                  <Card title="Proof Overview">
                    <InfoTable items={overviewItems} column={1} />
                  </Card>
                </Col>
                <Col xs={24} lg={12}>
                  <Card title="Transaction Reference">
                    <InfoTable items={txItems} column={1} />
                  </Card>
                </Col>
              </Row>
            ),
          },
          {
            key: 'chained',
            label: 'Chained Proof (L1/L2/L3)',
            children: (
              <ProofChainDiagram
                chainedProof={undefined}
                loading={false}
              />
            ),
          },
          {
            key: 'governance',
            label: 'Governance',
            children: (
              <GovernanceLevelCard
                levels={p.governance_levels}
                loading={false}
              />
            ),
          },
          {
            key: 'merkle',
            label: 'Merkle Proof',
            children: (
              <MerkleTreeVisualization
                merkleProof={
                  p.merkle_root
                    ? {
                        merkle_root: p.merkle_root,
                        leaf_hash: p.leaf_hash || '',
                        leaf_index: p.leaf_index || 0,
                        merkle_path: [],
                      }
                    : undefined
                }
              />
            ),
          },
          {
            key: 'anchor',
            label: 'Anchor',
            children: (
              <AnchorProgressTracker
                anchor={
                  p.anchor_tx_hash
                    ? {
                        target_chain: p.anchor_chain || 'unknown',
                        anchor_tx_hash: p.anchor_tx_hash,
                        anchor_block_number: p.anchor_block_number || 0,
                        confirmations: 12,
                      }
                    : undefined
                }
              />
            ),
          },
          {
            key: 'attestations',
            label: 'Attestations',
            children: <AttestationList attestations={p.attestations} />,
          },
          {
            key: 'custody',
            label: 'Custody Chain',
            children: custody.data ? (
              <Card title="Custody Chain">
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Text>Chain Length: </Text>
                    <Text strong>{custody.data.chain_length}</Text>
                    {custody.data.chain_valid ? (
                      <Tag color="green" style={{ marginLeft: 8 }}>Valid</Tag>
                    ) : (
                      <Tag color="red" style={{ marginLeft: 8 }}>Invalid</Tag>
                    )}
                  </div>
                  {custody.data.events.map((event, index) => (
                    <Card key={event.event_id} size="small">
                      <Space direction="vertical">
                        <Text strong>{index + 1}. {event.event_type.toUpperCase()}</Text>
                        <Text type="secondary">{new Date(event.event_timestamp).toLocaleString()}</Text>
                        <Text type="secondary">
                          Actor: {event.actor_type}
                          {event.actor_id && ` (${event.actor_id.slice(0, 16)}...)`}
                        </Text>
                        <Text code style={{ fontSize: 10 }}>Hash: {event.current_hash.slice(0, 32)}...</Text>
                      </Space>
                    </Card>
                  ))}
                </Space>
              </Card>
            ) : (
              <Card loading={custody.loading}>
                <Text type="secondary">Loading custody chain...</Text>
              </Card>
            ),
          },
          {
            key: 'download',
            label: 'Download',
            children: <BundleDownloader proofId={p.proof_id} />,
          },
        ]}
      />
    </div>
  );
}

export default ProofDetail;
