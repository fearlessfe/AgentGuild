import type { Revision } from "./reviews.types";

export type RevisionSelectorProps = {
  revisions: Revision[];
  selected: string;
  onSelect: (id: string) => void;
  label?: string;
};

export function RevisionSelector({ revisions, selected, onSelect, label = "Revision" }: RevisionSelectorProps) {
  return (
    <div className="revision-selector">
      <label htmlFor="revision-select">{label}</label>
      <select
        id="revision-select"
        value={selected}
        onChange={(e) => onSelect(e.target.value)}
        aria-label={label}
      >
        {revisions.map((revision) => (
          <option key={revision.id} value={revision.id}>
            {revision.label}
          </option>
        ))}
      </select>
    </div>
  );
}
