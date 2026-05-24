import * as React from "react";

import { cn } from "@/lib/utils";

function Input({
  className,
  type,
  inputSize = "default",
  startIcon,
  endIcon,
  ...props
}: React.ComponentProps<"input"> & {
  /** Compact height (h-7) for toolbars and filters */
  inputSize?: "default" | "compact";
  /** Icon rendered on the left inside the input */
  startIcon?: React.ReactNode;
  /** Icon rendered on the right inside the input */
  endIcon?: React.ReactNode;
}) {
  const hasStartIcon = !!startIcon;
  const hasEndIcon = !!endIcon;

  return (
    <div className="relative w-full">
      {hasStartIcon && (
        <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] [&_svg]:size-4">
          {startIcon}
        </span>
      )}
      <input
        type={type}
        data-slot="input"
        data-size={inputSize}
        className={cn(
          "w-full min-w-0 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-2 text-sm text-[var(--text-primary)] shadow-xs transition-[color,box-shadow] outline-none placeholder:text-[var(--text-muted)] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
          "focus-visible:border-[var(--accent-primary)] focus-visible:ring-1 focus-visible:ring-[var(--accent-primary)]",
          "aria-invalid:border-[var(--danger)]",
          inputSize === "compact" ? "h-7 text-xs" : "h-9",
          hasStartIcon && "pl-8",
          hasEndIcon && "pr-8",
          className,
        )}
        {...props}
      />
      {hasEndIcon && (
        <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] [&_svg]:size-4">
          {endIcon}
        </span>
      )}
    </div>
  );
}

export { Input };
