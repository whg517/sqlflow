import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  ShieldX,
  FileQuestion,
  ServerCrash,
  AlertTriangle,
} from "lucide-react";

interface ErrorPageProps {
  code?: 403 | 404 | 500;
  title?: string;
  message?: string;
}

export default function ErrorPage({
  code,
  title,
  message,
}: ErrorPageProps) {
  const navigate = useNavigate();

  if (code === 403) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
        <ShieldX size={64} className="text-[var(--risk-high)]" />
        <h1 className="text-2xl font-bold text-[var(--text-primary)]">403</h1>
        <p className="text-sm text-[var(--text-secondary)]">
          {message ?? "您没有访问此页面的权限"}
        </p>
        <Button variant="outline" onClick={() => navigate(-1)}>
          返回上一页
        </Button>
      </div>
    );
  }

  if (code === 500) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
        <ServerCrash size={64} className="text-[var(--risk-high)]" />
        <h1 className="text-2xl font-bold text-[var(--text-primary)]">
          {title ?? "服务器错误"}
        </h1>
        <p className="text-sm text-[var(--text-secondary)]">
          {message ?? "服务器遇到了问题，请稍后重试"}
        </p>
        <div className="flex gap-3">
          <Button variant="outline" onClick={() => window.location.reload()}>
            刷新页面
          </Button>
          <Button variant="outline" onClick={() => navigate("/query")}>
            返回首页
          </Button>
        </div>
      </div>
    );
  }

  if (code === 404) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
        <FileQuestion size={64} className="text-[var(--text-muted)]" />
        <h1 className="text-2xl font-bold text-[var(--text-primary)]">404</h1>
        <p className="text-sm text-[var(--text-secondary)]">
          {message ?? "页面不存在或已被移除"}
        </p>
        <Button variant="outline" onClick={() => navigate("/query")}>
          返回首页
        </Button>
      </div>
    );
  }

  // Generic / unexpected error
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
      <AlertTriangle size={64} className="text-[var(--risk-medium)]" />
      <h1 className="text-2xl font-bold text-[var(--text-primary)]">
        {title ?? "出了点问题"}
      </h1>
      <p className="max-w-md text-center text-sm text-[var(--text-secondary)]">
        {message ?? "发生了意外错误，请稍后重试"}
      </p>
      <div className="flex gap-3">
        <Button variant="outline" onClick={() => window.location.reload()}>
          刷新页面
        </Button>
        <Button variant="outline" onClick={() => navigate("/query")}>
          返回首页
        </Button>
      </div>
    </div>
  );
}
