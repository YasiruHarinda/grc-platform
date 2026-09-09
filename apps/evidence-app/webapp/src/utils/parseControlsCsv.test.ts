import { describe, expect, test } from "vitest";
import { parseControlsCsv } from "./parseControlsCsv";

describe("parseControlsCsv", () => {
  test("a well formed file produces one row per data line with the reference copied exactly", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,Access is reviewed quarterly.",
      "CC1.2,Backups are tested monthly.",
    ].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(unusableRowCount).toBe(0);
    expect(rows).toHaveLength(2);
    expect(rows[0].reference).toBe("CC1.1");
    expect(rows[1].reference).toBe("CC1.2");
  });

  test("a description shorter than the limit becomes the title unchanged, with no ellipsis", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,Backups are tested monthly.",
    ].join("\n");

    const { rows } = parseControlsCsv(csv);

    expect(rows[0].title).toBe("Backups are tested monthly.");
    expect(rows[0].title).not.toContain("…");
  });

  test("a description longer than the limit is cut at a word boundary and ends in an ellipsis", () => {
    // 20 words of "alphaN " (6 chars each) followed by a period, 140
    // characters long with its only sentence break at the very end, so the
    // whole thing is one sentence that has to be cut. The nearest space at
    // or before character 120 falls at index 118, so the cut keeps whole
    // words only.
    const words = Array.from({ length: 20 }, (_, i) => `alpha${i % 3}`);
    const description = `${words.join(" ")}.`;
    const csv = ["Control Number,Control Description from PY Report", `CC1.1,${description}`].join(
      "\n"
    );

    const { rows } = parseControlsCsv(csv);

    expect(rows[0].title).toBe(
      "alpha0 alpha1 alpha2 alpha0 alpha1 alpha2 alpha0 alpha1 alpha2 alpha0 alpha1 alpha2 alpha0 alpha1 alpha2 alpha0 alpha1…"
    );
    // The cut lands on a space in the original text, never inside a word.
    expect(description[118]).toBe(" ");
  });

  test("a multi sentence description yields only its first sentence as the title", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,Access is reviewed quarterly. Evidence is retained for one year.",
    ].join("\n");

    const { rows } = parseControlsCsv(csv);

    expect(rows[0].title).toBe("Access is reviewed quarterly.");
    expect(rows[0].description).toBe(
      "Access is reviewed quarterly. Evidence is retained for one year."
    );
  });

  test("a first sentence longer than the limit is still cut and still produces a title, so no row can ever require human input", () => {
    // One unbroken 130 character word, no space anywhere in it, so there is
    // no word boundary to cut at inside the limit at all.
    const oneLongWord = "a".repeat(130);
    const csv = [
      "Control Number,Control Description from PY Report",
      `CC1.1,${oneLongWord}.`,
    ].join("\n");

    const { rows } = parseControlsCsv(csv);

    expect(rows).toHaveLength(1);
    expect(rows[0].title).toBe(`${"a".repeat(119)}…`);
    expect(rows[0].title.length).toBe(120);
  });

  test("the full description is preserved on the row regardless of what the title became", () => {
    const oneLongWord = "a".repeat(130);
    const csv = [
      "Control Number,Control Description from PY Report",
      `CC1.1,${oneLongWord}.`,
    ].join("\n");

    const { rows } = parseControlsCsv(csv);

    expect(rows[0].description).toBe(`${oneLongWord}.`);
  });

  test("a quoted cell containing a comma is parsed as one value", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      'CC2.1,"Access, including privileged access, is reviewed quarterly."',
    ].join("\n");

    const { rows } = parseControlsCsv(csv);

    expect(rows).toHaveLength(1);
    expect(rows[0].description).toBe("Access, including privileged access, is reviewed quarterly.");
  });

  test("a quoted cell containing a line break is parsed as one value and does not split the row", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      'CC3.1,"Access is reviewed.\nEvidence is retained for one year."',
      "CC3.2,Backups are tested monthly.",
    ].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(unusableRowCount).toBe(0);
    expect(rows).toHaveLength(2);
    expect(rows[0].reference).toBe("CC3.1");
    expect(rows[0].description).toBe("Access is reviewed. Evidence is retained for one year.");
    expect(rows[1].reference).toBe("CC3.2");
  });

  test("headers with stray spaces or different capitalisation still match", () => {
    const csv = [
      "  Control Number  , control description from py report ",
      "CC1.1,Access is reviewed quarterly.",
    ].join("\n");

    const { rows, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(rows).toHaveLength(1);
    expect(rows[0].reference).toBe("CC1.1");
    expect(rows[0].title).toBe("Access is reviewed quarterly.");
  });

  test("a missing required column produces an error naming both expected columns, and no rows", () => {
    const csv = ["Control Number,Some Other Column", "CC1.1,foo"].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(rows).toEqual([]);
    expect(unusableRowCount).toBe(0);
    expect(error).toContain("Control Number");
    expect(error).toContain("Control Description from PY Report");
  });

  test("a row with a blank control number is reported as unusable rather than returned", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      ",Access is reviewed quarterly.",
      "CC1.2,Backups are tested monthly.",
    ].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(rows).toHaveLength(1);
    expect(rows[0].reference).toBe("CC1.2");
    expect(unusableRowCount).toBe(1);
  });

  test("a row with a blank description is reported as unusable rather than returned", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,",
      "CC1.2,Backups are tested monthly.",
    ].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(rows).toHaveLength(1);
    expect(rows[0].reference).toBe("CC1.2");
    expect(unusableRowCount).toBe(1);
  });

  test("a file with headers and no data rows returns no rows and no error", () => {
    const csv = "Control Number,Control Description from PY Report\n";

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(rows).toEqual([]);
    expect(unusableRowCount).toBe(0);
    expect(error).toBeNull();
  });

  test("an empty file returns a plain error", () => {
    const { rows, unusableRowCount, error } = parseControlsCsv("");

    expect(rows).toEqual([]);
    expect(unusableRowCount).toBe(0);
    expect(error).toBe("The file is empty.");
  });

  test("a reference repeated in the same file is kept once and the repeat counted as unusable", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,Access is reviewed quarterly.",
      "CC1.2,Backups are tested monthly.",
      "CC1.1,Access is reviewed quarterly.",
    ].join("\n");

    const { rows, unusableRowCount, error } = parseControlsCsv(csv);

    expect(error).toBeNull();
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.reference)).toEqual(["CC1.1", "CC1.2"]);
    expect(unusableRowCount).toBe(1);
  });

  test("a repeat differing only in spacing or casing is still treated as the same reference", () => {
    const csv = [
      "Control Number,Control Description from PY Report",
      "CC1.1,Access is reviewed quarterly.",
      " cc1.1 ,Access is reviewed quarterly.",
    ].join("\n");

    const { rows, unusableRowCount } = parseControlsCsv(csv);

    expect(rows).toHaveLength(1);
    expect(rows[0].reference).toBe("CC1.1");
    expect(unusableRowCount).toBe(1);
  });

  test("accented characters, curly quotes and bullet characters survive parsing unchanged", () => {
    const description = "Café passwords use “strong” policies • naïve users are re trained.";
    const csv = ["Control Number,Control Description from PY Report", `CC1.1,"${description}"`].join(
      "\n"
    );

    const { rows } = parseControlsCsv(csv);

    expect(rows[0].description).toBe(description);
  });
});
