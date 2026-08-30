import { ExternalLink } from "@/ui";
export interface SearchResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

export function SearchResults({ results }: { results: SearchResult[] }) {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
      {results.map((r) => (
        <ExternalLink
          key={r.url}
          href={r.url}
          title={r.url}
          className="group flex flex-col gap-1.5 rounded-md bg-sunken px-3.5 py-3 no-underline transition-colors duration-[var(--dur-fast)] ease-out hover:bg-hover"
        >
          <div className="flex items-center gap-1.5 font-mono text-ui-sm text-fg-muted">
            <span className="grid h-3.5 w-3.5 shrink-0 place-items-center rounded-2xs bg-surface-3 font-sans text-ui-2xs font-semibold text-fg-muted transition-colors group-hover:text-fg">
              {(r.domain[0] ?? "?").toUpperCase()}
            </span>
            <span className="truncate">{r.domain}</span>
          </div>
          <div className="line-clamp-2 text-ui-md font-semibold leading-snug text-fg">
            {r.title}
          </div>
          <div className="line-clamp-3 text-ui-md leading-body text-fg-muted">{r.snippet}</div>
        </ExternalLink>
      ))}
    </div>
  );
}
