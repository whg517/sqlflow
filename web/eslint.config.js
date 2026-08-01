import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

// Cross-feature dependencies that are allowed, with the reason for each.
//
// The frontend mirrors the backend's vertical split: web/src/features/<name>
// matches internal/<name>, and the same rule applies — a feature may always
// import from @/shared, but reaching into a sibling is a decision to couple two
// slices and has to be written down. `internal/arch` enforces the backend half;
// the generated no-restricted-imports rules below enforce this half.
//
// Prefer lifting the shared piece into @/shared over adding an entry here.
const allowedFeatureEdges = {
  audit: {
    ops: '审计详情展示关联的 Git 提交信息',
    query: '审计条目跳转到对应查询',
    ticket: '审计条目跳转到对应工单',
  },
  ops: {
    security: '设置页承载脱敏规则管理',
    ticket: '设置页承载审批策略与 SLA 配置',
  },
  query: {
    security: '编辑器与状态栏标注敏感表',
    ticket: '查询页展示 AI 评审结论',
  },
  ticket: {
    ops: '工单关联 Git 提交',
    query: '工单详情复用查询结果展示',
  },
}

const featureNames = ['audit', 'datasource', 'iam', 'notify', 'ops', 'query', 'security', 'ticket']

// One override per feature: everything under features/<name> may import from
// @/shared and from its declared partners, and nothing else under @/features.
const featureBoundaries = featureNames.map((feature) => ({
  files: [`src/features/${feature}/**/*.{ts,tsx}`],
  rules: {
    'no-restricted-imports': ['error', {
      patterns: featureNames
        .filter((other) => other !== feature && !allowedFeatureEdges[feature]?.[other])
        .map((other) => ({
          group: [`@/features/${other}/*`, `@/features/${other}`],
          message:
            `${feature} 不应依赖 ${other}：请把共用部分提到 @/shared，` +
            `或在 eslint.config.js 的 allowedFeatureEdges 中登记理由。`,
        })),
    }],
  },
}))

export default defineConfig([
  globalIgnores(['dist', 'node_modules', 'playwright-report']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      globals: globals.browser,
      parser: tseslint.parser,
    },
    rules: {
      // Let @typescript-eslint/no-unused-vars handle unused vars (smarter about TS types)
      'no-unused-vars': 'off',
      'no-console': 'warn',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': 'warn',
      // set-state-in-effect: too many legitimate useEffect+fetchData patterns;
      // downgrade to warn until refactored to useRequest hooks or similar
      'react-hooks/set-state-in-effect': 'warn',
      'react-refresh/only-export-components': ['warn', { allowExportNames: ['default'] }],
    },
  },
  ...featureBoundaries,
])
