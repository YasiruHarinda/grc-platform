import { describe, expect, test } from "vitest";
import { resolveHoverText } from "./resolveHoverText";

describe("resolveHoverText", () => {
  test("a Control with a description returns the description, not the title", () => {
    const control = {
      title: "Access is reviewed on a quarterly basis by the system owner",
      description: "Access rights are reviewed every quarter and any unused accounts are removed.",
    };

    expect(resolveHoverText(control)).toBe(
      "Access rights are reviewed every quarter and any unused accounts are removed."
    );
  });

  test("a Product with a description returns the description, not the name", () => {
    const product = {
      name: "Choreo",
      description: "The internal platform used to build and run the evidence app.",
    };

    expect(resolveHoverText(product)).toBe(
      "The internal platform used to build and run the evidence app."
    );
  });

  test("a Framework with a null description falls back to its name", () => {
    const framework = { name: "SOC 2", description: null };

    expect(resolveHoverText(framework)).toBe("SOC 2");
  });

  test("a Control with an empty string description falls back to its title", () => {
    const control = { title: "Backups are tested monthly", description: "" };

    expect(resolveHoverText(control)).toBe("Backups are tested monthly");
  });

  test("a Control with a description made only of whitespace falls back to its title", () => {
    const control = { title: "Backups are tested monthly", description: "   " };

    expect(resolveHoverText(control)).toBe("Backups are tested monthly");
  });

  test("a Control with no description and no title returns an empty string rather than throwing", () => {
    const control = { title: "", description: undefined };

    expect(resolveHoverText(control)).toBe("");
  });

  test("a Control with an absent description property falls back to its title", () => {
    const control = { title: "CA-20" };

    expect(resolveHoverText(control)).toBe("CA-20");
  });
});
