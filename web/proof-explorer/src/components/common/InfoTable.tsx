// Info Table Component (adapted from Accumulate Explorer)
import React from 'react';
import { Descriptions } from 'antd';

interface InfoItem {
  label: string;
  value: React.ReactNode;
  span?: number;
}

interface InfoTableProps {
  items: InfoItem[];
  title?: string;
  column?: number;
  bordered?: boolean;
}

export function InfoTable({
  items,
  title,
  column = 2,
  bordered = true,
}: InfoTableProps) {
  return (
    <Descriptions
      title={title}
      bordered={bordered}
      column={column}
      size="small"
    >
      {items.map((item, index) => (
        <Descriptions.Item
          key={index}
          label={item.label}
          span={item.span || 1}
        >
          {item.value}
        </Descriptions.Item>
      ))}
    </Descriptions>
  );
}

export default InfoTable;
