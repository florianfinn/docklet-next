/** Route of the container page. */
export function containerHref(id: string): string {
  return `/containers/${encodeURIComponent(id)}`;
}
