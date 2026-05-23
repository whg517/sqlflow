import { api } from './client'

// --- Types ---

export interface Comment {
  id: number
  order_id: number
  user_id: number
  username: string
  content: string
  parent_id: number
  created_at: string
}

export interface CommentListResponse {
  code: number
  message: string
  data: Comment[]
}

export interface CommentResponse {
  code: number
  message: string
  data: Comment
}

// --- API Functions ---

export async function listComments(orderId: number): Promise<CommentListResponse> {
  return api.get<CommentListResponse>(`/tickets/${orderId}/comments`)
}

export async function createComment(
  orderId: number,
  content: string,
  parentId?: number,
): Promise<CommentResponse> {
  return api.post<CommentResponse>(`/tickets/${orderId}/comments`, {
    content,
    parent_id: parentId || 0,
  })
}

export async function deleteComment(commentId: number): Promise<CommentResponse> {
  return api.del<CommentResponse>(`/comments/${commentId}`)
}

// --- Helpers ---

export function formatCommentTime(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const diff = now.getTime() - d.getTime()

  // Less than 1 minute
  if (diff < 60_000) return '刚刚'
  // Less than 1 hour
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  // Less than 24 hours
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  // Less than 7 days
  if (diff < 604_800_000) return `${Math.floor(diff / 86_400_000)} 天前`

  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hour = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${month}-${day} ${hour}:${min}`
}
