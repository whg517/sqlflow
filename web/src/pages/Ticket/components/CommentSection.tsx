import { useState, useEffect, useCallback, useRef } from 'react'
import { MessageSquare, Send, Trash2, CornerDownRight, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Separator } from '@/components/ui/separator'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  listComments,
  createComment,
  deleteComment,
  formatCommentTime,
  type Comment,
} from '@/api/comment'

interface CommentSectionProps {
  orderId: number
  currentUserId: number
  currentUserRole: string
}

export default function CommentSection({
  orderId,
  currentUserId,
  currentUserRole,
}: CommentSectionProps) {
  const [comments, setComments] = useState<Comment[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [content, setContent] = useState('')
  const [replyTo, setReplyTo] = useState<Comment | null>(null)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const fetchComments = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listComments(orderId)
      setComments(res.data || [])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '获取评论失败')
    } finally {
      setLoading(false)
    }
  }, [orderId])

  useEffect(() => {
    if (orderId > 0) {
      fetchComments()
    }
  }, [orderId, fetchComments])

  // Auto-scroll to bottom when comments change
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [comments])

  async function handleSubmit() {
    const text = content.trim()
    if (!text) return

    setSubmitting(true)
    try {
      const res = await createComment(orderId, text, replyTo?.id)
      setComments((prev) => [...prev, res.data])
      setContent('')
      setReplyTo(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '发送评论失败')
    } finally {
      setSubmitting(false)
    }
  }

  function handleReply(comment: Comment) {
    setReplyTo(comment)
    // Focus the textarea
    const textarea = document.querySelector<HTMLTextAreaElement>('[data-comment-input]')
    textarea?.focus()
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleSubmit()
    }
    if (e.key === 'Escape' && replyTo) {
      setReplyTo(null)
    }
  }

  async function handleDelete(commentId: number) {
    setDeletingId(commentId)
    try {
      await deleteComment(commentId)
      setComments((prev) => prev.filter((c) => c.id !== commentId && c.parent_id !== commentId))
      toast.success('评论已删除')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除评论失败')
    } finally {
      setDeletingId(null)
    }
  }

  function canDelete(comment: Comment): boolean {
    return comment.user_id === currentUserId || currentUserRole === 'admin' || currentUserRole === 'dba'
  }

  // Build a tree structure: top-level comments and their replies
  const topLevelComments = comments.filter((c) => c.parent_id === 0)
  const repliesMap = new Map<number, Comment[]>()
  comments.filter((c) => c.parent_id > 0).forEach((c) => {
    const existing = repliesMap.get(c.parent_id) || []
    existing.push(c)
    repliesMap.set(c.parent_id, existing)
  })

  function getInitials(name: string): string {
    return (name || '?').charAt(0).toUpperCase()
  }

  function renderComment(comment: Comment, isReply: boolean = false) {
    const replies = repliesMap.get(comment.id) || []
    const isDeleting = deletingId === comment.id

    return (
      <div key={comment.id} className={isReply ? 'ml-8 mt-2' : ''}>
        <div className="group flex gap-3 py-2">
          <Avatar size="sm" className="mt-0.5 shrink-0">
            <AvatarFallback className="bg-[var(--accent-primary)] text-white text-[10px]">
              {getInitials(comment.username)}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-[var(--text-primary)]">
                {comment.username || `用户#${comment.user_id}`}
              </span>
              <span className="text-[10px] text-[var(--text-muted)]">
                {formatCommentTime(comment.created_at)}
              </span>
            </div>
            <p className="mt-0.5 text-xs leading-relaxed text-[var(--text-secondary)] whitespace-pre-wrap break-words">
              {comment.content}
            </p>
            <div className="mt-1 flex items-center gap-2 opacity-0 transition-opacity group-hover:opacity-100">
              <button
                className="inline-flex items-center gap-1 text-[10px] text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
                onClick={() => handleReply(comment)}
              >
                <CornerDownRight size={10} />
                回复
              </button>
              {canDelete(comment) && (
                <button
                  className="inline-flex items-center gap-1 text-[10px] text-[var(--text-muted)] hover:text-red-400 transition-colors"
                  onClick={() => handleDelete(comment.id)}
                  disabled={isDeleting}
                >
                  {isDeleting ? <Loader2 size={10} className="animate-spin" /> : <Trash2 size={10} />}
                  删除
                </button>
              )}
            </div>
          </div>
        </div>
        {/* Render replies */}
        {replies.map((reply) => renderComment(reply, true))}
      </div>
    )
  }

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-center gap-2 pb-2">
        <MessageSquare size={14} className="text-[var(--text-muted)]" />
        <span className="text-xs font-medium text-[var(--text-secondary)]">
          评论 ({comments.length})
        </span>
      </div>

      {/* Reply indicator */}
      {replyTo && (
        <div className="mb-2 flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-1.5">
          <CornerDownRight size={12} className="text-[var(--text-muted)]" />
          <span className="text-[10px] text-[var(--text-muted)]">
            回复 {replyTo.username || `用户#${replyTo.user_id}`}
          </span>
          <button
            className="ml-auto text-[10px] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            onClick={() => setReplyTo(null)}
          >
            ✕
          </button>
        </div>
      )}

      <Separator className="bg-[var(--border-default)]" />

      {/* Comment list */}
      <div ref={scrollRef} className="max-h-[280px] overflow-y-auto py-1">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 size={16} className="animate-spin text-[var(--text-muted)]" />
          </div>
        ) : comments.length === 0 ? (
          <div className="py-6 text-center text-xs text-[var(--text-muted)]">
            暂无评论，参与讨论吧
          </div>
        ) : (
          topLevelComments.map((comment) => renderComment(comment))
        )}
      </div>

      <Separator className="bg-[var(--border-default)]" />

      {/* Input area */}
      <div className="pt-2">
        <div className="flex gap-2">
          <Textarea
            data-comment-input
            value={content}
            onChange={(e) => setContent(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={replyTo ? `回复 ${replyTo.username}...` : '输入评论... (Ctrl+Enter 发送)'}
            className="min-h-[60px] max-h-[120px] resize-none border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
            disabled={submitting}
          />
          <Button
            size="sm"
            className="h-auto shrink-0 self-end bg-[var(--accent-primary)] px-3 text-white hover:bg-[var(--accent-hover)]"
            onClick={handleSubmit}
            disabled={submitting || !content.trim()}
          >
            {submitting ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <Send size={14} />
            )}
          </Button>
        </div>
      </div>
    </div>
  )
}
