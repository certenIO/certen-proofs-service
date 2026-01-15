// Bundle Downloader - Download and verify proof bundles
import React, { useState } from 'react';
import { Card, Button, Typography, Space, message, Modal, Spin, Alert } from 'antd';
import { RiDownloadLine, RiCheckboxCircleFill, RiEyeLine, RiFileCopyLine } from 'react-icons/ri';
import proofApi from '../../api/proofApi';
import { CertenProofBundle, BundleVerificationResult } from '../../types/proof';

const { Text, Title, Paragraph } = Typography;

interface BundleDownloaderProps {
  proofId: string;
}

export function BundleDownloader({ proofId }: BundleDownloaderProps) {
  const [loading, setLoading] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const [verification, setVerification] = useState<BundleVerificationResult | null>(null);
  const [bundle, setBundle] = useState<CertenProofBundle | null>(null);
  const [showBundleModal, setShowBundleModal] = useState(false);

  const handleDownloadJSON = async () => {
    setLoading(true);
    try {
      const bundleData = await proofApi.downloadBundle(proofId);
      setBundle(bundleData);

      // Trigger download
      const blob = new Blob([JSON.stringify(bundleData, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `certen-proof-${proofId}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      message.success('Bundle downloaded successfully');
    } catch (error) {
      message.error(`Failed to download: ${error}`);
    } finally {
      setLoading(false);
    }
  };

  const handleDownloadGzip = async () => {
    setLoading(true);
    try {
      const blob = await proofApi.downloadBundleBlob(proofId);

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `certen-proof-${proofId}.json.gz`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      message.success('Compressed bundle downloaded');
    } catch (error) {
      message.error(`Failed to download: ${error}`);
    } finally {
      setLoading(false);
    }
  };

  const handleVerify = async () => {
    setVerifying(true);
    try {
      const result = await proofApi.verifyBundle(proofId);
      setVerification(result);
    } catch (error) {
      message.error(`Verification failed: ${error}`);
    } finally {
      setVerifying(false);
    }
  };

  const handlePreview = async () => {
    if (!bundle) {
      setLoading(true);
      try {
        const bundleData = await proofApi.downloadBundle(proofId);
        setBundle(bundleData);
      } catch (error) {
        message.error(`Failed to load bundle: ${error}`);
        setLoading(false);
        return;
      }
      setLoading(false);
    }
    setShowBundleModal(true);
  };

  const copyBundleToClipboard = async () => {
    if (bundle) {
      try {
        await navigator.clipboard.writeText(JSON.stringify(bundle, null, 2));
        message.success('Bundle copied to clipboard');
      } catch {
        message.error('Failed to copy');
      }
    }
  };

  return (
    <Card
      title={
        <Space>
          <RiDownloadLine />
          <Title level={5} style={{ margin: 0 }}>
            Download Bundle
          </Title>
        </Space>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {/* Description */}
        <Paragraph type="secondary">
          Download a self-contained verification bundle that includes all four proof
          components (Merkle, Anchor, Chained, Governance) for offline verification.
        </Paragraph>

        {/* Download Buttons */}
        <Space wrap>
          <Button
            type="primary"
            icon={<RiDownloadLine />}
            onClick={handleDownloadJSON}
            loading={loading}
          >
            Download JSON
          </Button>
          <Button
            icon={<RiDownloadLine />}
            onClick={handleDownloadGzip}
            loading={loading}
          >
            Download Compressed (.gz)
          </Button>
          <Button
            icon={<RiEyeLine />}
            onClick={handlePreview}
            loading={loading}
          >
            Preview
          </Button>
          <Button
            icon={<RiCheckboxCircleFill />}
            onClick={handleVerify}
            loading={verifying}
          >
            Verify Integrity
          </Button>
        </Space>

        {/* Verification Result */}
        {verification && (
          <Alert
            type={verification.bundle_valid ? 'success' : 'error'}
            showIcon
            message={verification.bundle_valid ? 'Bundle Verified' : 'Verification Failed'}
            description={
              <Space direction="vertical" size="small">
                <div>
                  <Text strong>Components:</Text>
                  <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                    <li>
                      Chained Proof:{' '}
                      {verification.components.chained_proof ? '\u2713' : '\u2717'}
                    </li>
                    <li>
                      Governance Proof:{' '}
                      {verification.components.governance_proof ? '\u2713' : '\u2717'}
                    </li>
                    <li>
                      Merkle Proof:{' '}
                      {verification.components.merkle_proof ? '\u2713' : '\u2717'}
                    </li>
                    <li>
                      Anchor Reference:{' '}
                      {verification.components.anchor_reference ? '\u2713' : '\u2717'}
                    </li>
                  </ul>
                </div>
                <div>
                  <Text strong>Integrity:</Text>
                  <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                    <li>
                      Hash Valid: {verification.integrity.hash_valid ? '\u2713' : '\u2717'}
                    </li>
                    <li>
                      Format Valid:{' '}
                      {verification.integrity.format_valid ? '\u2713' : '\u2717'}
                    </li>
                    <li>
                      Attestations Valid:{' '}
                      {verification.integrity.attestations_valid ? '\u2713' : '\u2717'}
                    </li>
                  </ul>
                </div>
              </Space>
            }
          />
        )}
      </Space>

      {/* Bundle Preview Modal */}
      <Modal
        title="Bundle Preview"
        open={showBundleModal}
        onCancel={() => setShowBundleModal(false)}
        width={800}
        footer={[
          <Button key="copy" icon={<RiFileCopyLine />} onClick={copyBundleToClipboard}>
            Copy to Clipboard
          </Button>,
          <Button key="close" onClick={() => setShowBundleModal(false)}>
            Close
          </Button>,
        ]}
      >
        {bundle ? (
          <pre
            style={{
              maxHeight: 500,
              overflow: 'auto',
              backgroundColor: 'rgba(230, 126, 34, 0.05)',
              padding: 16,
              borderRadius: 8,
              fontSize: 11,
              border: '1px solid rgba(230, 126, 34, 0.15)',
              color: '#ffffff',
            }}
          >
            {JSON.stringify(bundle, null, 2)}
          </pre>
        ) : (
          <Spin />
        )}
      </Modal>
    </Card>
  );
}

export default BundleDownloader;
