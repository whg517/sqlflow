# SF-FEAT0029 查询结果体验增强

> 状态：待评审
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoks4muEN

## 需求概述

查询结果宽表体验优化：冻结列、列宽拖拽、虚拟滚动、多列排序。

## 评审结论

待评审。

## 技术方案

### 方案选型

| 方案 | 说明 |
|------|------|
| **虚拟滚动** | @tanstack/react-virtual 替代全量渲染，5000+ 行场景不卡顿 |
| **冻结列** | @tanstack/react-table 内置 sticky columns 支持 |
| **列宽拖拽** | @tanstack/react-table 内置 column resize 支持 |
| **列排序** | @tanstack/react-table 已有 getSortedRowModel，扩展为多列排序 |

### 列宽偏好持久化

- 方案 A：localStorage（轻量，仅当前浏览器）
- 方案 B：后端用户配置表（跨设备同步）
- **建议方案 A**，后续如需要再升级到 B

### 改动范围

| 文件 | 变更 |
|------|------|
| web/src/pages/Query.tsx | ResultTable 组件升级 |
| web/src/components/ResultTable.tsx | 集成虚拟滚动 + 冻结列 + 列宽拖拽 + 多列排序 |
| web/package.json | 新增 @tanstack/react-virtual |

### 前端已有基础

- ResultTable 已使用 @tanstack/react-table
- 已有 getSortedRowModel（单列排序）

## 设计规范

- 冻结列：默认冻结行号列 + 第一列，用户可配置
- 列宽拖拽：最小 60px，双击自适应内容宽度
- 虚拟滚动：行高固定 36px，可视区域外不渲染 DOM

## 验收标准

1. 60 列+场景横向滚动流畅，冻结列跟随
2. 5000 行结果集不卡顿（P95 渲染时间 < 100ms）
3. 列宽拖拽生效，偏好可持久化
4. 多列排序正确（Shift+Click 叠加排序）
5. 不影响现有功能（分页、导出等）

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
