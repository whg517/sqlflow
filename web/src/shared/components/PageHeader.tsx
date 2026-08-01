import type { ReactNode } from "react";

interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
  meta?: ReactNode;
}

export default function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  meta,
}: PageHeaderProps) {
  return (
    <div className="flex flex-col gap-5 border-b border-[var(--border-subtle)] pb-6 xl:flex-row xl:items-end xl:justify-between">
      <div className="min-w-0">
        {eyebrow && (
          <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--accent-primary)]">
            <span className="h-px w-5 bg-[var(--accent-primary)]" />
            {eyebrow}
          </div>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-[26px] font-semibold leading-tight tracking-[-0.025em] text-[var(--text-primary)]">
            {title}
          </h1>
          {meta}
        </div>
        {description && (
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--text-secondary)]">
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
