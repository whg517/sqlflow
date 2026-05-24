import { useState } from "react";
import { toast } from "sonner";
import { KeyRound } from "lucide-react";
import { api } from "@/api/client";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface FieldErrors {
  oldPassword?: string;
  newPassword?: string;
  confirmPassword?: string;
}

function validatePassword(v: string): string | undefined {
  if (v.length < 8) return "密码长度至少 8 个字符";
  if (v.length > 128) return "密码长度不能超过 128 个字符";
  if (!/[a-zA-Z]/.test(v)) return "密码必须包含至少一个字母";
  if (!/[0-9]/.test(v)) return "密码必须包含至少一个数字";
  return undefined;
}

export default function ChangePasswordDialog({ open, onOpenChange }: Props) {
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [errors, setErrors] = useState<FieldErrors>({});
  const [submitting, setSubmitting] = useState(false);

  function validate(): FieldErrors {
    const e: FieldErrors = {};
    if (!oldPassword) e.oldPassword = "请输入当前密码";
    const npErr = validatePassword(newPassword);
    if (!newPassword) e.newPassword = "请输入新密码";
    else if (npErr) e.newPassword = npErr;
    if (!confirmPassword) e.confirmPassword = "请确认新密码";
    else if (newPassword !== confirmPassword)
      e.confirmPassword = "两次输入的密码不一致";
    return e;
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length > 0) return;

    setSubmitting(true);
    try {
      await api.put("/auth/password", {
        old_password: oldPassword,
        new_password: newPassword,
      });
      toast.success("密码修改成功");
      resetAndClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "密码修改失败，请重试";
      setErrors({ oldPassword: msg });
    } finally {
      setSubmitting(false);
    }
  }

  function resetAndClose() {
    setOldPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setErrors({});
    onOpenChange(false);
  }

  function handleNewPasswordBlur() {
    if (newPassword) {
      const e = validatePassword(newPassword);
      setErrors((prev) => ({ ...prev, newPassword: e }));
    }
  }

  function handleConfirmPasswordBlur() {
    if (confirmPassword && newPassword !== confirmPassword) {
      setErrors((prev) => ({
        ...prev,
        confirmPassword: "两次输入的密码不一致",
      }));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* §4.4/4.9: max-w-md */}
      <DialogContent
        className="sm:max-w-md"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          {/* §4.9: text-lg font-semibold */}
          <DialogTitle className="flex items-center gap-2 text-lg font-semibold">
            <KeyRound size={18} />
            修改密码
          </DialogTitle>
          {/* §4.9: text-sm text-secondary */}
          <DialogDescription className="text-sm text-[var(--text-secondary)]">
            请输入当前密码并设置新密码
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="grid gap-4">
          {/* 当前密码 */}
          <div className="grid gap-1.5">
            <Label htmlFor="old-pwd">当前密码</Label>
            <Input
              id="old-pwd"
              type="password"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              placeholder="请输入当前密码"
            />
            {errors.oldPassword && (
              <p className="text-xs text-[var(--danger)]">
                {errors.oldPassword}
              </p>
            )}
          </div>

          {/* 新密码 */}
          <div className="grid gap-1.5">
            <Label htmlFor="new-pwd">新密码</Label>
            <Input
              id="new-pwd"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              onBlur={handleNewPasswordBlur}
              placeholder="8-128 字符，需包含字母和数字"
            />
            {errors.newPassword && (
              <p className="text-xs text-[var(--danger)]">
                {errors.newPassword}
              </p>
            )}
          </div>

          {/* 确认新密码 */}
          <div className="grid gap-1.5">
            <Label htmlFor="confirm-pwd">确认新密码</Label>
            <Input
              id="confirm-pwd"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              onBlur={handleConfirmPasswordBlur}
              placeholder="再次输入新密码"
            />
            {errors.confirmPassword && (
              <p className="text-xs text-[var(--danger)]">
                {errors.confirmPassword}
              </p>
            )}
          </div>

          {/* §4.9: footer right-aligned, gap-2, cancel (outline) + confirm (accent-primary) */}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={resetAndClose}
              disabled={submitting}
            >
              取消
            </Button>
            <Button type="submit" loading={submitting}>
              {submitting ? "保存中..." : "保存修改"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
