import { useMemo, useState } from "react";
import { ChevronRight, ChevronDown } from "lucide-react";
import type { QueryResult } from "@/features/query/api/query";

// AggregationView renders a driver-native aggregation payload.
//
// Aggregations have no fixed column set — they are a tree of named buckets
// whose depth depends on the query — so the table view cannot show them.
// Bucket lists, the common case, get a real table; anything else falls back to
// an expandable tree so nothing is silently hidden.

interface Bucket {
  key: unknown;
  doc_count?: number;
  [k: string]: unknown;
}

interface NamedBuckets {
  name: string;
  buckets: Bucket[];
}

/** collectBucketGroups finds every `{ name: { buckets: [...] } }` in the tree. */
function collectBucketGroups(node: unknown, path: string[] = []): NamedBuckets[] {
  if (node === null || typeof node !== "object") return [];

  const found: NamedBuckets[] = [];
  for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
    if (value === null || typeof value !== "object") continue;
    const record = value as Record<string, unknown>;
    if (Array.isArray(record.buckets)) {
      found.push({
        name: [...path, key].join(" › "),
        buckets: record.buckets as Bucket[],
      });
    }
    found.push(...collectBucketGroups(value, [...path, key]));
  }
  return found;
}

function BucketTable({ group }: { group: NamedBuckets }) {
  // Buckets carry sub-aggregations as extra keys; show the scalar ones as
  // columns and leave nested structures to the raw tree below.
  const extraColumns = useMemo(() => {
    const cols = new Set<string>();
    for (const b of group.buckets) {
      for (const [k, v] of Object.entries(b)) {
        if (k === "key" || k === "doc_count") continue;
        if (v !== null && typeof v === "object") continue;
        cols.add(k);
      }
    }
    return [...cols].sort();
  }, [group.buckets]);

  return (
    <div className="mb-4">
      <div className="mb-1.5 text-xs font-medium text-[var(--text-secondary)]">
        {group.name}
        <span className="ml-2 text-[var(--text-muted)]">
          {group.buckets.length} 个分桶
        </span>
      </div>
      <div className="overflow-x-auto rounded-md border border-[var(--border-default)]">
        <table className="w-full text-sm">
          <thead className="bg-[var(--bg-elevated)]">
            <tr>
              <th className="px-3 py-1.5 text-left font-medium text-[var(--text-secondary)]">
                key
              </th>
              <th className="px-3 py-1.5 text-left font-medium text-[var(--text-secondary)]">
                doc_count
              </th>
              {extraColumns.map((c) => (
                <th
                  key={c}
                  className="px-3 py-1.5 text-left font-medium text-[var(--text-secondary)]"
                >
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {group.buckets.map((b, i) => (
              <tr
                key={i}
                className="border-t border-[var(--border-default)] text-[var(--text-primary)]"
              >
                <td className="px-3 py-1.5">{String(b.key ?? "")}</td>
                <td className="px-3 py-1.5">{b.doc_count ?? ""}</td>
                {extraColumns.map((c) => (
                  <td key={c} className="px-3 py-1.5">
                    {b[c] === null || b[c] === undefined ? "" : String(b[c])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function JsonNode({
  label,
  value,
  depth,
}: {
  label: string;
  value: unknown;
  depth: number;
}) {
  const [open, setOpen] = useState(depth < 2);
  const isBranch = value !== null && typeof value === "object";

  if (!isBranch) {
    return (
      <div className="flex gap-2 py-0.5" style={{ paddingLeft: depth * 14 }}>
        <span className="text-[var(--text-muted)]">{label}:</span>
        <span className="text-[var(--text-primary)]">
          {value === null ? "null" : String(value)}
        </span>
      </div>
    );
  }

  const entries = Array.isArray(value)
    ? value.map((v, i) => [String(i), v] as const)
    : Object.entries(value as Record<string, unknown>);

  return (
    <div style={{ paddingLeft: depth * 14 }}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 py-0.5 text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span>{label}</span>
        <span className="text-[var(--text-muted)]">
          {Array.isArray(value) ? `[${entries.length}]` : `{${entries.length}}`}
        </span>
      </button>
      {open &&
        entries.map(([k, v]) => (
          <JsonNode key={k} label={k} value={v} depth={depth + 1} />
        ))}
    </div>
  );
}

export default function AggregationView({
  result,
}: {
  result: QueryResult | null;
}) {
  const aggregations = result?.aggregations;

  const groups = useMemo(
    () => (aggregations ? collectBucketGroups(aggregations) : []),
    [aggregations],
  );

  if (!aggregations) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-[var(--text-muted)]">
        无聚合结果
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto p-3 font-mono text-xs">
      {groups.map((g) => (
        <BucketTable key={g.name} group={g} />
      ))}

      <div className="mt-2">
        <div className="mb-1.5 text-xs font-medium text-[var(--text-secondary)]">
          原始聚合结果
        </div>
        <div className="rounded-md border border-[var(--border-default)] p-2">
          <JsonNode label="aggregations" value={aggregations} depth={0} />
        </div>
      </div>
    </div>
  );
}
