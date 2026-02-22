import { ChevronLeft, ChevronRight } from "lucide-react";

interface PaginationProps {
  page: number;
  totalPages: number;
  total: number;
  onPageChange: (page: number) => void;
  label?: string;
}

export function Pagination({
  page,
  totalPages,
  total,
  onPageChange,
  label = "entries",
}: PaginationProps) {
  if (totalPages <= 1) return null;

  return (
    <div
      className="sticky z-10 flex items-center justify-between mb-4 py-2 px-1 -mx-1 bg-surface-0/95 backdrop-blur-sm text-sm text-text-2"
      style={{ top: "-25px" }}
    >
      <span>
        {total.toLocaleString()} {label}
      </span>
      <nav className="flex items-center gap-2" aria-label="Pagination">
        <button
          onClick={() => onPageChange(Math.max(0, page - 1))}
          disabled={page === 0}
          aria-label="Previous page"
          className="p-1.5 rounded-lg border border-border-1 hover:bg-surface-2 disabled:opacity-30 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          <ChevronLeft className="w-4 h-4" aria-hidden="true" />
        </button>
        <span className="tabular-nums" aria-current="page">
          {page + 1} / {totalPages}
        </span>
        <button
          onClick={() => onPageChange(Math.min(totalPages - 1, page + 1))}
          disabled={page >= totalPages - 1}
          aria-label="Next page"
          className="p-1.5 rounded-lg border border-border-1 hover:bg-surface-2 disabled:opacity-30 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          <ChevronRight className="w-4 h-4" aria-hidden="true" />
        </button>
      </nav>
    </div>
  );
}
