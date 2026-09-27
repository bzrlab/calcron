import type { ReactNode } from "react";

export type Column<T> = {
  key: string;
  label: string;
  render: (row: T) => ReactNode;
  className?: string;
};

export function DataTable<T extends Record<string, unknown>>({
  columns,
  rows,
  loading,
  empty = "No rows.",
  onRowClick,
  selectedId,
}: {
  columns: Column<T>[];
  rows: T[];
  loading?: boolean;
  empty?: ReactNode;
  onRowClick?: (row: T) => void;
  selectedId?: string;
}) {
  return (
    <div className="overflow-x-auto rounded-box border border-base-300 bg-base-100">
      <table className="table table-sm">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.key} className={`text-xs uppercase tracking-wider ${c.className ?? ""}`}>
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading && rows.length === 0 && (
            <tr>
              <td colSpan={columns.length} className="py-8 text-center text-base-content/50">
                <span className="loading loading-spinner loading-sm" />
              </td>
            </tr>
          )}
          {!loading && rows.length === 0 && (
            <tr>
              <td colSpan={columns.length} className="py-8 text-center text-base-content/50">
                {empty}
              </td>
            </tr>
          )}
          {rows.map((row, i) => (
            <tr
              key={String(row.id ?? i)}
              className={`${onRowClick ? "cursor-pointer hover" : ""} ${selectedId !== undefined && row.id === selectedId ? "bg-base-200" : ""}`}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
            >
              {columns.map((c) => (
                <td key={c.key} className={c.className}>
                  {c.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
