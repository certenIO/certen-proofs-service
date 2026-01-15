// Hash Display Component
import React, { useState } from 'react';
import { Typography, Tooltip, Button, Space, message } from 'antd';
import { RiFileCopyLine, RiExternalLinkLine } from 'react-icons/ri';

const { Text } = Typography;

interface HashDisplayProps {
  hash: string;
  truncate?: boolean;
  truncateLength?: number;
  copyable?: boolean;
  linkTo?: string;
  label?: string;
  mono?: boolean;
}

export function HashDisplay({
  hash,
  truncate = true,
  truncateLength = 8,
  copyable = true,
  linkTo,
  label,
  mono = true,
}: HashDisplayProps) {
  const displayHash = truncate
    ? `${hash.slice(0, truncateLength)}...${hash.slice(-truncateLength)}`
    : hash;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(hash);
      message.success('Copied to clipboard');
    } catch {
      message.error('Failed to copy');
    }
  };

  return (
    <Space size="small">
      {label && <Text type="secondary">{label}:</Text>}
      <Tooltip title={hash}>
        <Text code={mono} style={{ fontFamily: mono ? 'monospace' : undefined }}>
          {displayHash}
        </Text>
      </Tooltip>
      {copyable && (
        <Button
          type="text"
          size="small"
          icon={<RiFileCopyLine />}
          onClick={handleCopy}
        />
      )}
      {linkTo && (
        <Button
          type="text"
          size="small"
          icon={<RiExternalLinkLine />}
          href={linkTo}
          target="_blank"
        />
      )}
    </Space>
  );
}

interface AccountLinkProps {
  url: string;
  truncate?: boolean;
}

export function AccountLink({ url, truncate = true }: AccountLinkProps) {
  const displayUrl = truncate && url.length > 40
    ? `${url.slice(0, 20)}...${url.slice(-15)}`
    : url;

  return (
    <Tooltip title={url}>
      <a href={`/account/${encodeURIComponent(url)}`}>
        <Text code style={{ fontFamily: 'monospace' }}>
          {displayUrl}
        </Text>
      </a>
    </Tooltip>
  );
}

export default HashDisplay;
