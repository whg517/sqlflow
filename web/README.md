# SQLFlow Web

SQLFlow 前端是 React + TypeScript + Vite 单页应用。生产构建由 Go 服务嵌入，开发时由 Vite 提供 HMR 并把 API 代理到本地后端。

## 开发

需要 Node.js 22+ 和 npm：

```bash
npm install
npm run dev
```

默认访问 `http://localhost:5173`。后端应另行运行在 `http://localhost:8080`。

## 命令

```bash
npm run build          # TypeScript 检查 + 生产构建
npm run lint           # ESLint
npm run test           # Vitest 单次运行
npm run test:watch     # Vitest watch
npm run test:coverage  # 覆盖率报告
npm run preview        # 本地预览生产构建
```

## 目录边界

```text
src/api/          HTTP 客户端、接口模块和传输类型
src/pages/        页面级编排和路由入口
src/components/   可复用业务组件与 UI 基础组件
src/hooks/        可复用副作用和交互状态
src/store/        跨页面客户端状态
src/lib/          通用前端工具
src/test/         测试环境和共享测试设施
```

页面通过 `src/App.tsx` 中的 `lazyPage` 懒加载。新 API 调用进入 `src/api`，新页面必须处理加载、空数据、错误和 403；后端权限始终是最终安全边界。

## 相关文档

- [项目入口](../README.md)
- [前端架构](../docs/ARCHITECTURE.md#9-前端架构)
- [暗色主题 Token](docs/dark-mode-design-tokens.md)
