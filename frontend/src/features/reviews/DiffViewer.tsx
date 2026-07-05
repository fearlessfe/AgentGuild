import { Fragment, useState } from "react";
import type { DiffLine, FileDiff, LineComment } from "./reviews.types";

type DiffMode = "split" | "unified";

type SelectedLine = {
  key: string;
  lineNumber: number;
  side: "left" | "right";
};

function lineMarker(type: DiffLine["type"]): string {
  switch (type) {
    case "add":
      return "+";
    case "remove":
      return "−";
    default:
      return " ";
  }
}

function commentsForLine(comments: LineComment[], lineNumber: number, side: "left" | "right"): LineComment[] {
  return comments.filter((c) => c.line_number === lineNumber && c.side === side);
}

export function DiffViewer({
  diff,
  comments,
  onAddComment,
}: {
  diff: FileDiff;
  comments: LineComment[];
  onAddComment: (line: number, side: "left" | "right", text: string) => void;
}) {
  const [mode, setMode] = useState<DiffMode>("split");
  const [selected, setSelected] = useState<SelectedLine | null>(null);
  const [draft, setDraft] = useState("");

  function selectLine(key: string, lineNumber: number | undefined, side: "left" | "right") {
    if (lineNumber === undefined) return;
    setSelected({ key, lineNumber, side });
    setDraft("");
  }

  function submitComment() {
    if (!selected || draft.trim() === "") return;
    onAddComment(selected.lineNumber, selected.side, draft.trim());
    setSelected(null);
    setDraft("");
  }

  const allLines: { hunkIndex: number; lineIndex: number; line: DiffLine }[] = [];
  diff.hunks.forEach((hunk, hunkIndex) => {
    hunk.lines.forEach((line, lineIndex) => {
      allLines.push({ hunkIndex, lineIndex, line });
    });
  });

  return (
    <div className="diff-viewer">
      <div className="diff-toolbar" role="toolbar" aria-label="Diff 视图模式">
        <button
          type="button"
          aria-pressed={mode === "split"}
          className={mode === "split" ? "active" : undefined}
          onClick={() => setMode("split")}
        >
          Split
        </button>
        <button
          type="button"
          aria-pressed={mode === "unified"}
          className={mode === "unified" ? "active" : undefined}
          onClick={() => setMode("unified")}
        >
          Unified
        </button>
        <span className="diff-path" title={diff.path}>
          {diff.path}
        </span>
      </div>

      {mode === "unified" ? (
        <table className="diff-table unified">
          <tbody>
            {allLines.map(({ hunkIndex, lineIndex, line }) => {
              const keyBase = `${hunkIndex}-${lineIndex}`;
              const leftKey = `${keyBase}-left`;
              const rightKey = `${keyBase}-right`;
              const leftComments = line.old_line ? commentsForLine(comments, line.old_line, "left") : [];
              const rightComments = line.new_line ? commentsForLine(comments, line.new_line, "right") : [];
              return (
                <Fragment key={keyBase}>
                  <tr className={`diff-line ${line.type}`}>
                    <td className="line-number old">
                      {line.old_line !== undefined ? (
                        <button
                          type="button"
                          className={selected?.key === leftKey ? "selected" : undefined}
                          aria-label={`在左侧第 ${line.old_line} 行添加评论`}
                          onClick={() => selectLine(leftKey, line.old_line, "left")}
                        >
                          {line.old_line}
                        </button>
                      ) : null}
                    </td>
                    <td className="line-number new">
                      {line.new_line !== undefined ? (
                        <button
                          type="button"
                          className={selected?.key === rightKey ? "selected" : undefined}
                          aria-label={`在右侧第 ${line.new_line} 行添加评论`}
                          onClick={() => selectLine(rightKey, line.new_line, "right")}
                        >
                          {line.new_line}
                        </button>
                      ) : null}
                    </td>
                    <td className="line-marker" aria-hidden="true">
                      {lineMarker(line.type)}
                    </td>
                    <td className="line-content">
                      <pre>{line.text}</pre>
                    </td>
                  </tr>
                  {leftComments.length > 0 ? (
                    <tr className="comment-row">
                      <td colSpan={4}>
                        <CommentList items={leftComments} sideLabel="左侧" />
                      </td>
                    </tr>
                  ) : null}
                  {rightComments.length > 0 ? (
                    <tr className="comment-row">
                      <td colSpan={4}>
                        <CommentList items={rightComments} sideLabel="右侧" />
                      </td>
                    </tr>
                  ) : null}
                  {selected && (selected.key === leftKey || selected.key === rightKey) ? (
                    <CommentFormRow
                      key={`form-${selected.key}`}
                      selected={selected}
                      draft={draft}
                      onChange={setDraft}
                      onSubmit={submitComment}
                      onCancel={() => setSelected(null)}
                      colSpan={4}
                    />
                  ) : null}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      ) : (
        <table className="diff-table split">
          <thead>
            <tr>
              <th className="line-number">旧行</th>
              <th className="line-content">旧版本</th>
              <th className="line-number">新行</th>
              <th className="line-content">新版本</th>
            </tr>
          </thead>
          <tbody>
            {allLines.map(({ hunkIndex, lineIndex, line }) => {
              const keyBase = `${hunkIndex}-${lineIndex}`;
              const leftKey = `${keyBase}-left`;
              const rightKey = `${keyBase}-right`;
              const leftComments = line.old_line ? commentsForLine(comments, line.old_line, "left") : [];
              const rightComments = line.new_line ? commentsForLine(comments, line.new_line, "right") : [];
              return (
                <Fragment key={keyBase}>
                  <tr className={`diff-line ${line.type}`}>
                    <td className="line-number old">
                      {line.old_line !== undefined ? (
                        <button
                          type="button"
                          className={selected?.key === leftKey ? "selected" : undefined}
                          aria-label={`在左侧第 ${line.old_line} 行添加评论`}
                          onClick={() => selectLine(leftKey, line.old_line, "left")}
                        >
                          {line.old_line}
                        </button>
                      ) : null}
                    </td>
                    <td className={`line-content old ${line.type === "add" ? "empty" : ""}`}>
                      {line.type !== "add" ? <pre>{line.text}</pre> : null}
                    </td>
                    <td className="line-number new">
                      {line.new_line !== undefined ? (
                        <button
                          type="button"
                          className={selected?.key === rightKey ? "selected" : undefined}
                          aria-label={`在右侧第 ${line.new_line} 行添加评论`}
                          onClick={() => selectLine(rightKey, line.new_line, "right")}
                        >
                          {line.new_line}
                        </button>
                      ) : null}
                    </td>
                    <td className={`line-content new ${line.type === "remove" ? "empty" : ""}`}>
                      {line.type !== "remove" ? <pre>{line.text}</pre> : null}
                    </td>
                  </tr>
                  {leftComments.length > 0 ? (
                    <tr className="comment-row">
                      <td colSpan={2}>
                        <CommentList items={leftComments} sideLabel="左侧" />
                      </td>
                      <td colSpan={2} />
                    </tr>
                  ) : null}
                  {rightComments.length > 0 ? (
                    <tr className="comment-row">
                      <td colSpan={2} />
                      <td colSpan={2}>
                        <CommentList items={rightComments} sideLabel="右侧" />
                      </td>
                    </tr>
                  ) : null}
                  {selected && (selected.key === leftKey || selected.key === rightKey) ? (
                    <CommentFormRow
                      selected={selected}
                      draft={draft}
                      onChange={setDraft}
                      onSubmit={submitComment}
                      onCancel={() => setSelected(null)}
                      colSpan={4}
                    />
                  ) : null}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

function CommentList({ items, sideLabel }: { items: LineComment[]; sideLabel: string }) {
  return (
    <ul className="comment-list" aria-label={`${sideLabel}评论`}>
      {items.map((comment) => (
        <li key={comment.id} className="comment-item">
          <span className="comment-meta">{sideLabel} · 第 {comment.line_number} 行</span>
          <p>{comment.text}</p>
        </li>
      ))}
    </ul>
  );
}

function CommentFormRow({
  selected,
  draft,
  onChange,
  onSubmit,
  onCancel,
  colSpan,
}: {
  selected: SelectedLine;
  draft: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
  colSpan: number;
}) {
  return (
    <tr className="comment-form-row">
      <td colSpan={colSpan}>
        <div className="comment-form">
          <label htmlFor={`comment-draft-${selected.key}`}>
            在{selected.side === "left" ? "左侧" : "右侧"}第 {selected.lineNumber} 行添加评论
          </label>
          <textarea
            id={`comment-draft-${selected.key}`}
            value={draft}
            onChange={(e) => onChange(e.target.value)}
            rows={3}
            placeholder="输入评论…"
          />
          <div className="comment-actions">
            <button type="button" onClick={onSubmit} disabled={draft.trim() === ""}>
              添加评论
            </button>
            <button type="button" className="secondary" onClick={onCancel}>
              取消
            </button>
          </div>
        </div>
      </td>
    </tr>
  );
}
