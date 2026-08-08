// How a data source type is labelled and coloured.
//
// This is the one file allowed to name types, and it is exempted in
// eslint.config.js for that reason. Presentation needs a lookup table somewhere
// — a checker needs to know what it is looking for, and so does a legend — but
// the exemption is narrow on purpose: nothing here decides behaviour. Anything
// that changes what the app *does* reads the driver's own declaration instead,
// from GET /api/datasource-types or GET /api/datasources/:id/capabilities.
//
// An unknown type degrades to its own name and a neutral colour rather than
// disappearing, so a newly registered driver is usable before anyone adds a
// line here.

interface Presentation {
  label: string;
  dot: string;
  badge: string;
}

const presentation: Record<string, Presentation> = {
  mysql: {
    label: "MySQL",
    dot: "bg-blue-400",
    badge: "border-blue-500/30 bg-blue-500/10 text-blue-400",
  },
  postgresql: {
    label: "PostgreSQL",
    dot: "bg-sky-400",
    badge: "border-sky-500/30 bg-sky-500/10 text-sky-400",
  },
  sqlite: {
    label: "SQLite",
    dot: "bg-slate-400",
    badge: "border-slate-500/30 bg-slate-500/10 text-slate-400",
  },
  mongodb: {
    label: "MongoDB",
    dot: "bg-green-400",
    badge: "border-green-500/30 bg-green-500/10 text-green-400",
  },
  elasticsearch: {
    label: "Elasticsearch",
    dot: "bg-orange-400",
    badge: "border-orange-500/30 bg-orange-500/10 text-orange-400",
  },
};

const fallback: Presentation = {
  label: "",
  dot: "bg-zinc-400",
  badge: "border-zinc-500/30 bg-zinc-500/10 text-zinc-400",
};

/** datasourceLabel is the display name for a type, or the type itself. */
export function datasourceLabel(type: string): string {
  return presentation[type]?.label || type;
}

/** datasourceDot is the colour class for the small status dot in a list. */
export function datasourceDot(type: string): string {
  return presentation[type]?.dot ?? fallback.dot;
}

/** datasourceBadge is the colour class for a type badge. */
export function datasourceBadge(type: string): string {
  return presentation[type]?.badge ?? fallback.badge;
}
