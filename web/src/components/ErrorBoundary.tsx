import { Component, type ReactNode, type ErrorInfo } from "react";
import { AlertTriangle, RefreshCw, Home } from "lucide-react";
import { Button } from "@/components/ui/button";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  showHomeButton?: boolean;
  title?: string;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

export default class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // TODO: 接入 Sentry
    this.props.onError?.(error, errorInfo);
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null });
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)] p-8">
          <AlertTriangle size={64} className="text-[var(--risk-medium)]" />
          <h1 className="text-xl font-bold text-[var(--text-primary)]">
            {this.props.title ?? "页面出现了问题"}
          </h1>
          {this.state.error && (
            <p className="max-w-md text-center text-sm text-[var(--text-secondary)]">
              {this.state.error.message}
            </p>
          )}
          <div className="flex gap-3">
            <Button variant="outline" onClick={this.handleRetry}>
              <RefreshCw size={16} className="mr-1.5" />
              重试
            </Button>
            {this.props.showHomeButton !== false && (
              <Button variant="outline" asChild>
                <a href="/query">
                  <Home size={16} className="mr-1.5" />
                  返回首页
                </a>
              </Button>
            )}
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
