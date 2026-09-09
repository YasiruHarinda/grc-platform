// The one rule for what a hover box shows across the Control picker, the
// Product and Framework pickers, and the Catalogue page: the full
// description, or the record's title or name when it has no description to
// show. Kept structural, so the pickers' own, richer Control, Product and
// Framework types satisfy these without a cast.
export type HoverTextControl = { title: string; description?: string | null };
export type HoverTextProduct = { name: string; description?: string | null };
export type HoverTextFramework = { name: string; description?: string | null };

/**
 * Picks the text a hover box shows for a Control, a Product or a
 * Framework: the full description when there is one, otherwise the
 * record's title or name so the box is never empty.
 *
 * The Controls CSV importer builds a Control's title from the first
 * sentence of its description, shortened with a stored ellipsis. Where a
 * description exists, showing it alone avoids making the reader read that
 * same sentence twice. Where a record has no description at all, which is
 * possible for one added by hand, the title or name is the only thing left
 * to show — see spec #129.
 */
export function resolveHoverText(
  record: HoverTextControl | HoverTextProduct | HoverTextFramework
): string {
  const description = record.description?.trim();
  if (description) return description;
  return "title" in record ? record.title : record.name;
}
