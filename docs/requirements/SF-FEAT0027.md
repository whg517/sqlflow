# SF-FEAT0027 前端 Code-Splitting 路由级懒加载

> 状态：待评审
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMokv249yU

## 需求概述

使用 React.lazy() + Suspense 实现路由级懒加载，降低首屏 JS bundle 体积，提升加载性能。

## 评审结论

待评审。

## 技术方案

### 方案

- React.lazy() 包裹页面组件（Dashboard / Query / Ticket / Audit / Settings 等）
- Suspense fallback 使用 loading skeleton 组件
- Layout 公共部分保持同步加载
- Vite 自动 code-splitting（dynamic import）

### 改动范围

| 文件 | 变更 |
|------|------|
| App.tsx | 路由组件改为 React.lazy() |
| 新增 components/LoadingSkeleton.tsx | 骨架屏占位组件 |

### 不涉及

- 不改组件内部结构
- 不引入路由级 preload（暂不需要）

## 设计规范

- Skeleton 样式与现有深色/浅色主题一致
- 骨架屏尺寸参考各页面实际布局

## 验收标准

1. 首屏 JS bundle 降至 200KB 以下
2. 路由切换正常，无闪白屏
3. 骨架屏过渡自然
4. Lighthouse Performance 评分提升

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
