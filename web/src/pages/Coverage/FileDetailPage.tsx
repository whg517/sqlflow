import { useParams, useLocation, useNavigate, Link } from "react-router-dom";
import { ChevronRight, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useFileList } from "@/hooks/coverage";
import { FileTable } from "@/components/coverage/FileTable";

/**
 * CR fix: project name read from route state (passed by ModuleTable) or env.
 */
const FALLBACK_PROJECT = import.meta.env.VITE_COVERAGE_PROJECT ?? "sqlflow";

/** File-level coverage drill-down page with breadcrumb navigation. */
export default function FileDetailPage() {
  const { modulePath } = useParams<{ modulePath: string }>();
  const location = useLocation();
  const navigate = useNavigate();

  const decodedPath = decodeURIComponent(modulePath ?? "");
  const project =
    (location.state as { project?: string })?.project ?? FALLBACK_PROJECT;

  const { files, loading, error } = useFileList(project, decodedPath);

  const segments = decodedPath.split("/").filter(Boolean);

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header */}
      <div className="space-y-3">
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 text-[var(--text-secondary)]"
          onClick={() => navigate("/coverage/summary")}
        >
          <ArrowLeft size={14} />
          返回模块列表
        </Button>

        {/* Breadcrumb */}
        <nav className="flex items-center gap-1.5 text-sm" aria-label="Breadcrumb">
          <Link
            to="/coverage/summary"
            className="text-[var(--text-secondary)] transition-colors hover:text-[var(--text-primary)]"
          >
            覆盖度审计
          </Link>
          {segments.map((seg, idx) => (
            <span key={idx} className="flex items-center gap-1.5">
              <ChevronRight size={14} className="text-[var(--text-muted)]" />
              {idx === segments.length - 1 ? (
                <span className="font-medium text-[var(--text-primary)]">
                  {seg}
                </span>
              ) : (
                <span className="text-[var(--text-tertiary)]">{seg}</span>
              )}
            </span>
          ))}
        </nav>

        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          {decodedPath}
        </h1>
        <p className="text-sm text-[var(--text-secondary)]">文件级覆盖度明细</p>
      </div>

      {/* File table */}
      <Card>
        <CardContent className="p-5">
          <FileTable
            files={files}
            loading={loading}
            error={error}
            modulePath={decodedPath}
          />
        </CardContent>
      </Card>
    </div>
  );
}
