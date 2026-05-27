import { lazy, Suspense, type ComponentType } from "react";
import { Loader2 } from "lucide-react";
import ErrorBoundary from "./ErrorBoundary";

/**
 * 页面加载中 fallback —— 与现有设计风格一致
 */
function PageLoading() {
  return (
    <div className="flex h-full min-h-[200px] flex-col items-center justify-center gap-3">
      <Loader2 size={32} className="animate-spin text-[var(--text-secondary)]" />
      <span className="text-sm text-[var(--text-secondary)]">加载中…</span>
    </div>
  );
}

interface LazyPageOptions {
  /** ErrorBoundary 展示的标题 */
  title?: string;
}

/**
 * 将同步 import 转为 React.lazy 包装的懒加载组件。
 *
 * 用法:
 *   const DashboardPage = lazyPage(() => import("@/pages/Dashboard"), { title: "仪表盘页面出现了问题" });
 *
 * - 自动包裹 Suspense（Loading fallback）
 * - 自动包裹 ErrorBoundary（懒加载失败兜底）
 * - Vite 会将每个 lazy() 的 import 自动拆分为独立 chunk
 */
export function lazyPage(
  factory: () => Promise<{ default: ComponentType }>,
  options: LazyPageOptions = {},
) {
  const LazyComponent = lazy(factory);

  function WrappedPage() {
    return (
      <ErrorBoundary title={options.title}>
        <Suspense fallback={<PageLoading />}>
          <LazyComponent />
        </Suspense>
      </ErrorBoundary>
    );
  }

  WrappedPage.displayName = `LazyPage(${options.title ?? "Page"})`;

  return WrappedPage;
}
