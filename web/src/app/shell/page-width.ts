/**
 * Maximum content width per feature. On 16:9 and 21:9 screens only the
 * topology uses the whole width; lists with host blocks get a wide bound,
 * detail pages and dashboards a balanced one, forms a readable one. The page
 * header sits inside the same bound, so its actions align with the content.
 */
export type PageWidth = "full" | "lists" | "dashboard" | "detail" | "standard" | "narrow";

export const PAGE_MAX_WIDTH: Record<PageWidth, string | undefined> = {
  full: undefined,
  lists: "168rem",
  dashboard: "160rem",
  detail: "140rem",
  standard: "120rem",
  narrow: "88rem",
};

export function pageWidth(pathname: string): PageWidth {
  const [first, second] = pathname.split("/").filter(Boolean);
  switch (first) {
    case "networks":
      return "full";
    case "stacks":
    case "containers":
    case "images":
    case "volumes":
      return second ? "detail" : "lists";
    case "dashboard":
      return "dashboard";
    case "security":
    case "hosts":
    case "backups":
      return "standard";
    case "settings":
    case "profile":
    case "preview":
      return "narrow";
    default:
      return "standard";
  }
}
