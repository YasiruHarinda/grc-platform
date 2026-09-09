import Papa from "papaparse";

// The two columns this module reads. Named exactly as they appear in the
// SRE team's SOC 2 export — matched after trimming and lowercasing, never
// as an exact string, because that same file also carries a column called
// `Common or Prodcut specific ` (typo and trailing space both present),
// which is the shape of problem this guards against for next quarter's
// export rather than a fix for today's.
const CONTROL_NUMBER_COLUMN = "Control Number";
const CONTROL_DESCRIPTION_COLUMN = "Control Description from PY Report";

// Titles longer than this get cut at a word boundary and given a trailing
// ellipsis. Chosen by measuring the real 103 row file: cutting here left
// every title at 120 characters or under, no two titles alike, and no row
// needing a human to shorten it by hand.
const TITLE_LIMIT = 120;
const ELLIPSIS = "…";

export type ParsedControlRow = {
  reference: string;
  title: string;
  description: string;
};

export type ParseControlsCsvResult = {
  rows: ParsedControlRow[];
  unusableRowCount: number;
  error: string | null;
};

/**
 * Turns the text of a CSV export into the Controls it describes.
 *
 * Reads `Control Number` and `Control Description from PY Report`, and
 * discards the rest of the file's columns. Descriptions in the real export
 * carry line breaks and commas inside quoted cells, so this hands the whole
 * text to papaparse rather than splitting on commas or newlines by hand,
 * which would corrupt those rows silently.
 *
 * A row missing either column's value can't become a Control, so it is
 * counted in `unusableRowCount` rather than returned, and so is a second row
 * repeating a reference the file has already described. Every row that clears
 * that bar gets a title: the first sentence of its description, cut at a
 * word boundary and given a trailing ellipsis once it runs past
 * `TITLE_LIMIT` characters. That guarantee, that no row ever needs a human
 * to write its title by hand, is what lets a hundred-row file become a
 * hundred Controls in one pass.
 *
 * Plain data in, plain data out: no DOM, no network, nothing about where
 * the text came from or where the rows are going.
 */
export function parseControlsCsv(csvText: string): ParseControlsCsvResult {
  if (csvText.trim() === "") {
    return { rows: [], unusableRowCount: 0, error: "The file is empty." };
  }

  const parsed = Papa.parse<Record<string, string>>(csvText, {
    header: true,
    skipEmptyLines: true,
  });

  const fields = parsed.meta.fields ?? [];
  const numberKey = matchColumn(fields, CONTROL_NUMBER_COLUMN);
  const descriptionKey = matchColumn(fields, CONTROL_DESCRIPTION_COLUMN);

  if (!numberKey || !descriptionKey) {
    return {
      rows: [],
      unusableRowCount: 0,
      error: `Expected a "${CONTROL_NUMBER_COLUMN}" column and a "${CONTROL_DESCRIPTION_COLUMN}" column.`,
    };
  }

  const rows: ParsedControlRow[] = [];
  const seenRefs = new Set<string>();
  let unusableRowCount = 0;

  for (const record of parsed.data) {
    const reference = (record[numberKey] ?? "").trim();
    const description = normalizeWhitespace(record[descriptionKey] ?? "");

    // A row with no reference or no description can't become a usable
    // Control, so it is dropped and counted rather than pushed through with
    // a blank field standing in for something a human would have to fill.
    if (!reference || !description) {
      unusableRowCount += 1;
      continue;
    }

    // The same reference twice in one file describes one Control, not two.
    // A real export repeats a control across criteria rows, and nothing
    // downstream would catch it — the import dialog compares against what is
    // already stored, which the second copy isn't yet, so both would be
    // created and a re-import would then skip the pair as already there.
    // Matched on the lowercased reference, the same way that check is.
    const seenKey = reference.toLowerCase();
    if (seenRefs.has(seenKey)) {
      unusableRowCount += 1;
      continue;
    }
    seenRefs.add(seenKey);

    rows.push({ reference, title: buildTitle(description), description });
  }

  return { rows, unusableRowCount, error: null };
}

// Finds the header that matches `expected` once both sides are trimmed and
// lowercased, so stray spaces or a different capitalisation in the file
// still line up with the column this module is looking for.
function matchColumn(fields: string[], expected: string): string | undefined {
  const target = expected.toLowerCase();
  return fields.find((field) => field.trim().toLowerCase() === target);
}

// Collapses runs of whitespace, including the line breaks a quoted cell can
// carry, into single spaces, then trims the ends. Everything else in the
// text, accented letters, curly quotes, bullet characters, is left alone.
function normalizeWhitespace(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

// A cut title still has to fit in TITLE_LIMIT once the ellipsis is added,
// so the word boundary is looked for one character short of the limit.
const BODY_LIMIT = TITLE_LIMIT - ELLIPSIS.length;

// The title is the description's first sentence, shortened only when it
// would otherwise overrun the column it sits in. A cut always lands on a
// word boundary, so a shortened title never ends mid word, and the
// ellipsis is only added when a cut actually happened, so an untouched
// short description reads exactly as written.
function buildTitle(description: string): string {
  const sentence = firstSentence(description);
  if (sentence.length <= TITLE_LIMIT) return sentence;

  const boundary = sentence.lastIndexOf(" ", BODY_LIMIT);
  // No space at all inside the limit means one long unbroken word. There is
  // still no such thing as a row that can't get a title, so this falls back
  // to a hard cut instead of leaving the row without one.
  const cutAt = boundary > 0 ? boundary : BODY_LIMIT;
  return sentence.slice(0, cutAt).trimEnd() + ELLIPSIS;
}

// The text up to and including the first sentence-ending punctuation mark,
// or the whole description when it never finds one.
function firstSentence(description: string): string {
  const match = description.match(/^.*?[.!?](?=\s|$)/);
  return match ? match[0] : description;
}
