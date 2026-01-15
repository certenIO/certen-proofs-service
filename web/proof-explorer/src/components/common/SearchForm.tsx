// Search Form Component (adapted from Accumulate Explorer)
import React, { useState } from 'react';
import { Input, Button, Space, Radio, Form } from 'antd';
import { RiSearchLine } from 'react-icons/ri';
import { useNavigate } from 'react-router-dom';

type SearchType = 'tx' | 'account' | 'proof' | 'batch';

interface SearchFormProps {
  defaultType?: SearchType;
  onSearch?: (type: SearchType, value: string) => void;
}

export function SearchForm({ defaultType = 'tx', onSearch }: SearchFormProps) {
  const [searchType, setSearchType] = useState<SearchType>(defaultType);
  const [searchValue, setSearchValue] = useState('');
  const navigate = useNavigate();

  const handleSearch = () => {
    if (!searchValue.trim()) return;

    if (onSearch) {
      onSearch(searchType, searchValue);
      return;
    }

    // Default navigation
    switch (searchType) {
      case 'tx':
        navigate(`/proof/tx/${searchValue}`);
        break;
      case 'account':
        navigate(`/account/${encodeURIComponent(searchValue)}`);
        break;
      case 'proof':
        navigate(`/proof/${searchValue}`);
        break;
      case 'batch':
        navigate(`/batch/${searchValue}`);
        break;
    }
  };

  const getPlaceholder = () => {
    switch (searchType) {
      case 'tx':
        return 'Enter Accumulate transaction hash (64 hex chars)';
      case 'account':
        return 'Enter account URL (e.g., acc://example.acme/tokens)';
      case 'proof':
        return 'Enter proof ID (UUID)';
      case 'batch':
        return 'Enter batch ID (UUID)';
    }
  };

  return (
    <Form layout="vertical" className="search-form">
      <Form.Item>
        <Radio.Group
          value={searchType}
          onChange={(e) => setSearchType(e.target.value)}
          optionType="button"
          buttonStyle="solid"
        >
          <Radio.Button value="tx">TX Hash</Radio.Button>
          <Radio.Button value="account">Account</Radio.Button>
          <Radio.Button value="proof">Proof ID</Radio.Button>
          <Radio.Button value="batch">Batch ID</Radio.Button>
        </Radio.Group>
      </Form.Item>
      <Form.Item>
        <Space.Compact style={{ width: '100%' }}>
          <Input
            placeholder={getPlaceholder()}
            value={searchValue}
            onChange={(e) => setSearchValue(e.target.value)}
            onPressEnter={handleSearch}
            size="large"
          />
          <Button
            type="primary"
            icon={<RiSearchLine />}
            onClick={handleSearch}
            size="large"
          >
            Search
          </Button>
        </Space.Compact>
      </Form.Item>
    </Form>
  );
}

export default SearchForm;
