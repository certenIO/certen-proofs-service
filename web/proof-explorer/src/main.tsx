import React from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { ConfigProvider, theme } from 'antd';
import App from './App';
import './index.css';

// Certen Protocol Brand Theme
// Matching wallet-interface and authority-editor design system
const certenTheme = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Primary Colors - Certen Orange
    colorPrimary: '#E67E22',
    colorLink: '#E67E22',
    colorLinkHover: '#C76919',
    colorLinkActive: '#C76919',

    // Background Colors - Dark Navy/Purple
    colorBgBase: '#0f0f1e',
    colorBgContainer: '#1a1a2e',
    colorBgElevated: '#1a1a2e',
    colorBgLayout: '#0f0f1e',
    colorBgSpotlight: '#1a1a2e',

    // Text Colors
    colorText: '#ffffff',
    colorTextSecondary: '#94a3b8',
    colorTextTertiary: '#64748b',
    colorTextQuaternary: '#475569',

    // Border Colors - Orange tinted
    colorBorder: 'rgba(230, 126, 34, 0.15)',
    colorBorderSecondary: 'rgba(230, 126, 34, 0.1)',

    // Status Colors
    colorSuccess: '#10b981',
    colorWarning: '#f59e0b',
    colorError: '#ef4444',
    colorInfo: '#E67E22',

    // Typography
    fontFamily: '-apple-system, BlinkMacSystemFont, Inter, Segoe UI, sans-serif',
    fontSize: 14,
    fontSizeHeading1: 38,
    fontSizeHeading2: 30,
    fontSizeHeading3: 24,
    fontSizeHeading4: 20,
    fontSizeHeading5: 16,

    // Shape
    borderRadius: 8,
    borderRadiusLG: 12,
    borderRadiusSM: 6,

    // Spacing
    padding: 16,
    paddingLG: 24,
    paddingSM: 12,
    paddingXS: 8,

    // Effects
    boxShadow: '0 6px 16px 0 rgba(0, 0, 0, 0.3)',
    boxShadowSecondary: '0 3px 6px -4px rgba(0, 0, 0, 0.3)',
  },
  components: {
    Layout: {
      headerBg: '#1a1a2e',
      bodyBg: '#0f0f1e',
      footerBg: '#0f0f1e',
      siderBg: '#1a1a2e',
    },
    Menu: {
      darkItemBg: '#1a1a2e',
      darkSubMenuItemBg: '#0f0f1e',
      darkItemSelectedBg: 'rgba(230, 126, 34, 0.15)',
      darkItemHoverBg: 'rgba(230, 126, 34, 0.1)',
      darkItemSelectedColor: '#E67E22',
    },
    Card: {
      colorBgContainer: '#1a1a2e',
      colorBorderSecondary: 'rgba(230, 126, 34, 0.15)',
    },
    Table: {
      colorBgContainer: '#1a1a2e',
      headerBg: '#1a1a2e',
      headerColor: '#94a3b8',
      rowHoverBg: 'rgba(230, 126, 34, 0.05)',
      borderColor: 'rgba(230, 126, 34, 0.1)',
    },
    Button: {
      primaryColor: '#ffffff',
      fontWeight: 600,
    },
    Input: {
      colorBgContainer: 'rgba(255, 255, 255, 0.02)',
      colorBorder: 'rgba(230, 126, 34, 0.15)',
      hoverBorderColor: 'rgba(230, 126, 34, 0.3)',
      activeBorderColor: '#E67E22',
    },
    Select: {
      colorBgContainer: 'rgba(255, 255, 255, 0.02)',
      colorBorder: 'rgba(230, 126, 34, 0.15)',
      optionSelectedBg: 'rgba(230, 126, 34, 0.15)',
    },
    Tag: {
      defaultBg: 'rgba(230, 126, 34, 0.15)',
      defaultColor: '#ffffff',
    },
    Tabs: {
      inkBarColor: '#E67E22',
      itemSelectedColor: '#E67E22',
      itemHoverColor: '#C76919',
    },
    Alert: {
      colorInfoBg: 'rgba(230, 126, 34, 0.1)',
      colorInfoBorder: 'rgba(230, 126, 34, 0.2)',
    },
    Breadcrumb: {
      itemColor: '#94a3b8',
      lastItemColor: '#ffffff',
      linkColor: '#E67E22',
      linkHoverColor: '#C76919',
      separatorColor: '#64748b',
    },
    Statistic: {
      colorTextDescription: '#94a3b8',
      colorTextHeading: '#ffffff',
    },
    Progress: {
      defaultColor: '#E67E22',
    },
    Spin: {
      colorPrimary: '#E67E22',
    },
    Steps: {
      colorPrimary: '#E67E22',
    },
    Descriptions: {
      labelBg: 'rgba(230, 126, 34, 0.05)',
      contentColor: '#ffffff',
      labelColor: '#94a3b8',
    },
    Modal: {
      contentBg: '#1a1a2e',
      headerBg: '#1a1a2e',
      footerBg: '#1a1a2e',
    },
    Tooltip: {
      colorBgSpotlight: '#1a1a2e',
    },
    Pagination: {
      itemActiveBg: 'rgba(230, 126, 34, 0.15)',
    },
  },
};

const container = document.getElementById('root');
const root = createRoot(container!);

root.render(
  <React.StrictMode>
    <ConfigProvider theme={certenTheme}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConfigProvider>
  </React.StrictMode>
);
