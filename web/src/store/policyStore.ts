/**
 * Approval policy Zustand store — SF-FEAT0044
 * Manages policy CRUD + reorder + toggle
 */

import { create } from "zustand";
import { toast } from "sonner";
import type { ApprovalPolicy, PolicyCreateRequest, PolicyUpdateRequest } from "@/types/approval";
import * as policyApi from "@/api/policy";

interface PolicyState {
  policies: ApprovalPolicy[];
  loading: boolean;
  saving: boolean;
  error: string | null;
  editingPolicy: ApprovalPolicy | null;

  fetchPolicies: () => Promise<void>;
  createPolicy: (req: PolicyCreateRequest) => Promise<ApprovalPolicy>;
  updatePolicy: (id: number, req: PolicyUpdateRequest) => Promise<void>;
  deletePolicy: (id: number) => Promise<void>;
  togglePolicy: (id: number) => Promise<void>;
  reorderPolicies: (orderedIds: number[]) => Promise<void>;
  setEditingPolicy: (policy: ApprovalPolicy | null) => void;
  reset: () => void;
}

export const usePolicyStore = create<PolicyState>((set, get) => ({
  policies: [],
  loading: false,
  saving: false,
  error: null,
  editingPolicy: null,

  fetchPolicies: async () => {
    set({ loading: true, error: null });
    try {
      const res = await policyApi.listPolicies();
      set({ policies: res.data ?? [], loading: false });
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "获取策略列表失败",
        loading: false,
      });
    }
  },

  createPolicy: async (req: PolicyCreateRequest) => {
    set({ saving: true, error: null });
    try {
      const res = await policyApi.createPolicy(req);
      await get().fetchPolicies();
      set({ saving: false });
      toast.success(`策略「${req.name}」创建成功`);
      return res.data;
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "创建策略失败",
        saving: false,
      });
      throw err;
    }
  },

  updatePolicy: async (id: number, req: PolicyUpdateRequest) => {
    set({ saving: true, error: null });
    try {
      await policyApi.updatePolicy(id, req);
      await get().fetchPolicies();
      set({ saving: false });
      toast.success(`策略「${req.name}」已保存`);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "保存策略失败";
      if (msg.includes("409") || msg.includes("冲突") || msg.includes("conflict")) {
        toast.error("策略已被他人修改，请刷新后重试");
      } else {
        toast.error(msg);
      }
      set({ saving: false });
      throw err;
    }
  },

  deletePolicy: async (id: number) => {
    set({ saving: true, error: null });
    try {
      await policyApi.deletePolicy(id);
      await get().fetchPolicies();
      set({ saving: false, editingPolicy: null });
      toast.success("策略已删除");
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "删除策略失败",
        saving: false,
      });
      throw err;
    }
  },

  togglePolicy: async (id: number) => {
    const policy = get().policies.find((p) => p.id === id);
    if (!policy) return;
    const newEnabled = !policy.enabled;
    try {
      await policyApi.togglePolicy(id, newEnabled, policy.version);
      set((state) => ({
        policies: state.policies.map((p) =>
          p.id === id ? { ...p, enabled: newEnabled } : p,
        ),
      }));
      toast.success(newEnabled ? `策略「${policy.name}」已启用` : `策略「${policy.name}」已禁用`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    }
  },

  reorderPolicies: async (orderedIds: number[]) => {
    const prev = get().policies;
    const reordered = orderedIds
      .map((id, idx) => {
        const p = prev.find((item) => item.id === id);
        return p ? { ...p, priority: idx + 1 } : null;
      })
      .filter(Boolean) as ApprovalPolicy[];
    set({ policies: reordered });
    try {
      await policyApi.reorderPolicies({ ordered_ids: orderedIds });
      toast.success("排序已保存");
    } catch (err) {
      set({ policies: prev });
      toast.error(err instanceof Error ? err.message : "排序保存失败");
    }
  },

  setEditingPolicy: (policy: ApprovalPolicy | null) => {
    set({ editingPolicy: policy });
  },

  reset: () => {
    set({
      policies: [],
      loading: false,
      saving: false,
      error: null,
      editingPolicy: null,
    });
  },
}));
