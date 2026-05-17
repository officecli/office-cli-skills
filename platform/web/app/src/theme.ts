import { theme, type ThemeConfig } from 'antd'

export const appTheme: ThemeConfig = {
  algorithm: [theme.darkAlgorithm, theme.compactAlgorithm],
  token: {
    colorPrimary: '#5e6ad2',
    colorInfo: '#5e6ad2',
    colorSuccess: '#27a644',
    colorWarning: '#d89614',
    colorError: '#ff5c73',
    colorBgBase: '#0b0d12',
    colorBgContainer: '#11141b',
    colorBgElevated: '#161a23',
    colorBgLayout: '#0b0d12',
    colorBorder: '#252a35',
    colorBorderSecondary: '#1d222c',
    colorTextBase: '#f7f8f8',
    colorText: '#f7f8f8',
    colorTextSecondary: '#a6adbb',
    colorTextTertiary: '#707786',
    borderRadius: 8,
    borderRadiusLG: 12,
    borderRadiusSM: 6,
    fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    fontFamilyCode: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    wireframe: false,
  },
  components: {
    Layout: {
      bodyBg: '#0b0d12',
      headerBg: '#0f1219',
      siderBg: '#0f1219',
      triggerBg: '#161a23',
    },
    Card: {
      colorBgContainer: '#11141b',
      colorBorderSecondary: '#252a35',
      headerBg: '#11141b',
    },
    Menu: {
      darkItemBg: '#0f1219',
      darkSubMenuItemBg: '#0f1219',
      darkItemSelectedBg: '#1b2040',
      darkItemSelectedColor: '#ffffff',
      darkItemHoverBg: '#161a23',
    },
    Table: {
      headerBg: '#161a23',
      rowHoverBg: '#181d27',
      borderColor: '#252a35',
    },
    Button: {
      borderRadius: 8,
      controlHeight: 32,
    },
    Form: {
      labelColor: '#a6adbb',
    },
    Statistic: {
      titleFontSize: 12,
      contentFontSize: 28,
    },
  },
}
