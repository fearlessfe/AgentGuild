import type { LineComment as LineCommentType } from "./reviews.types";

export function LineComment({ comment }: { comment: LineCommentType }) {
  return (
    <div className="line-comment" data-testid="line-comment">
      <p className="line-comment-text">{comment.text}</p>
      <span className="line-comment-meta">
        {comment.side === "left" ? "左侧" : "右侧"} · 第 {comment.line_number} 行
      </span>
    </div>
  );
}
