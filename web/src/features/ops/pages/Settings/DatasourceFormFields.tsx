import { Input } from "@/shared/components/ui/input";
import { Label } from "@/shared/components/ui/label";
import { Switch } from "@/shared/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import type { DatasourceType } from "@/shared/datasource/types";
import { visibleFields, type FormValues } from "./datasourceForm";

interface Props {
  type: DatasourceType | undefined;
  values: FormValues;
  errors: Record<string, string>;
  editing: boolean;
  onChange: (name: string, value: string) => void;
}

/**
 * DatasourceFormFields renders whatever the selected driver says it needs.
 *
 * It knows nothing about any particular data source. Previously this markup was
 * inline in the Settings dialog and gated by six separate comparisons against
 * the type name, which is why adding a driver meant editing it — and why a
 * driver the form had not been told about could not be configured at all.
 */
export default function DatasourceFormFields({
  type,
  values,
  errors,
  editing,
  onChange,
}: Props) {
  const fields = visibleFields(type, values);
  if (fields.length === 0) return null;

  return (
    <div className="space-y-4">
      {fields.map((field) => {
        const value = values[field.name] ?? "";
        const error = errors[field.name];

        return (
          <div key={field.name} className="space-y-1.5">
            <Label className="text-[var(--text-secondary)]">
              {field.label}
              {field.secret && editing && (
                <span className="ml-1 font-normal text-[var(--text-muted)]">
                  (留空不修改)
                </span>
              )}
            </Label>

            {field.kind === "select" ? (
              <Select value={value} onValueChange={(v) => onChange(field.name, v)}>
                <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(field.options ?? []).map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : field.kind === "switch" ? (
              <div className="flex items-center gap-2">
                <Switch
                  checked={value === "true"}
                  onCheckedChange={(on) => onChange(field.name, on ? "true" : "false")}
                />
                <span className="text-xs text-[var(--text-muted)]">
                  {value === "true" ? "已开启" : "已关闭"}
                </span>
              </div>
            ) : (
              <Input
                type={
                  field.kind === "password"
                    ? "password"
                    : field.kind === "number"
                      ? "number"
                      : "text"
                }
                value={value}
                onChange={(e) => onChange(field.name, e.target.value)}
                placeholder={
                  field.secret && editing ? "留空不修改" : (field.placeholder ?? "")
                }
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
            )}

            {field.help && (
              <p className="text-xs text-[var(--text-muted)]">{field.help}</p>
            )}
            {error && <p className="text-xs text-red-400">{error}</p>}
          </div>
        );
      })}
    </div>
  );
}
