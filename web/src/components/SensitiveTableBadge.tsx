import { ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { SENSITIVITY_BADGE, type SensitiveTable } from "@/api/maskRule";

interface SensitiveTableBadgeProps {
  sensitivityLevel: SensitiveTable["sensitivity_level"];
  showLabel?: boolean;
  size?: "sm" | "md";
}

/**
 * Visual badge for sensitive tables with shield icon and red-tinted background.
 */
export function SensitiveTableBadge({
  sensitivityLevel,
  showLabel = false,
  size = "sm",
}: SensitiveTableBadgeProps) {
  const badge = SENSITIVITY_BADGE[sensitivityLevel] ?? SENSITIVITY_BADGE.medium;

  const sizeClasses =
    size === "sm" ? "px-1.5 py-0.5 text-[10px]" : "px-2 py-0.5 text-xs";

  return (
    <Badge className={`gap-0.5 border-0 ${badge.cls} ${sizeClasses}`}>
      <ShieldAlert size={size === "sm" ? 10 : 12} />
      {showLabel && <span>{badge.label}</span>}
    </Badge>
  );
}

interface SensitiveTableNameProps {
  tableName: string;
  sensitivityLevel: SensitiveTable["sensitivity_level"];
}

/**
 * Table name with red background label when the table is sensitive.
 */
export function SensitiveTableName({
  tableName,
  sensitivityLevel,
}: SensitiveTableNameProps) {
  const levelBg =
    sensitivityLevel === "high"
      ? "bg-red-500/20"
      : sensitivityLevel === "medium"
        ? "bg-red-500/15"
        : "bg-red-500/10";
  return (
    <span className="inline-flex items-center gap-1.5">
      <ShieldAlert size={13} className="shrink-0 text-red-400" />
      <span
        className={`rounded ${levelBg} px-1.5 py-0.5 font-medium text-[var(--text-primary)]`}
      >
        {tableName}
      </span>
    </span>
  );
}
