import React from 'react';
import { Routes, Route, Link } from 'react-router-dom';
import { Layout, Menu, Typography } from 'antd';
import { RiHomeLine, RiSearchLine } from 'react-icons/ri';

// Pages
import Dashboard from './pages/Dashboard';
import ProofDetail from './pages/ProofDetail';
import AccountTimeline from './pages/AccountTimeline';
import BatchInspector from './pages/BatchInspector';

// Assets
import logoSvg from './assets/logos/logo.svg';

const { Header, Content, Footer } = Layout;
const { Text } = Typography;

function App() {
  const menuItems = [
    {
      key: 'home',
      icon: <RiHomeLine />,
      label: <Link to="/">Dashboard</Link>,
    },
    {
      key: 'search',
      icon: <RiSearchLine />,
      label: <Link to="/">Search</Link>,
    },
  ];

  return (
    <Layout className="app-layout">
      <Header className="app-header">
        <Link to="/" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center' }}>
          <img src={logoSvg} alt="Certen" className="app-logo-img" />
        </Link>
        <span className="app-title">Proof Explorer</span>
        <Menu
          theme="dark"
          mode="horizontal"
          defaultSelectedKeys={['home']}
          items={menuItems}
          style={{ flex: 1, minWidth: 0, background: 'transparent', borderBottom: 'none' }}
        />
        <Text className="version-badge">v0.1.0</Text>
      </Header>

      <Content className="app-content">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/proof/tx/:txHash" element={<ProofDetail />} />
          <Route path="/proof/:proofId" element={<ProofDetail />} />
          <Route path="/account/:accountUrl" element={<AccountTimeline />} />
          <Route path="/batch/:batchId" element={<BatchInspector />} />
        </Routes>
      </Content>

      <Footer className="app-footer">
        Certen Protocol © {new Date().getFullYear()} — Cryptographic proof verification for Accumulate blockchain
      </Footer>
    </Layout>
  );
}

export default App;
