import type { FileDiff } from "./reviews.types";

export function FileTree({
  files,
  selected,
  onSelect,
}: {
  files: FileDiff[];
  selected: string;
  onSelect: (path: string) => void;
}) {
  if (files.length === 0) {
    return <div className="file-tree empty">没有可展示的文件变更</div>;
  }

  return (
    <ul className="file-tree" aria-label="文件树">
      {files.map((file) => (
        <li key={file.path}>
          <button
            type="button"
            className={selected === file.path ? "selected" : undefined}
            aria-pressed={selected === file.path}
            aria-label={`查看 ${file.path}`}
            onClick={() => onSelect(file.path)}
          >
            {file.path}
          </button>
        </li>
      ))}
    </ul>
  );
}
