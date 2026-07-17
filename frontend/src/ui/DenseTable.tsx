import type { ReactNode } from "react";

/* A thin wrapper around a semantic table that applies the design's dense-table
   styling. Rows and cells are provided as ReactNode so callers can embed chips,
   links or code freely. Use a `group` row to render a full-width section label. */

export type DenseCell = ReactNode;

export type DenseRow =
  | { group: ReactNode }
  | { cells: DenseCell[]; selected?: boolean; key?: string };

function isGroup(row: DenseRow): row is { group: ReactNode } {
  return "group" in row;
}

export function DenseTable({
  columns,
  rows,
  caption,
}: {
  columns: ReactNode[];
  rows: DenseRow[];
  caption?: string;
}) {
  return (
    <div className="dense-table-scroll" role="region" aria-label={caption || "数据表格"} tabIndex={0}>
      <table className="dense-table">
        {caption ? <caption className="visually-hidden">{caption}</caption> : null}
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th scope="col" key={index}>
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => {
            if (isGroup(row)) {
              return (
                <tr className="group-row" key={`group-${index}`}>
                  <td colSpan={columns.length}>{row.group}</td>
                </tr>
              );
            }
            return (
              <tr data-selected={row.selected ? "true" : "false"} key={row.key ?? index}>
                {row.cells.map((cell, cellIndex) => (
                  <td key={cellIndex}>{cell}</td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
