// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { describe, expect, it } from "vitest";
import { parseDateOnly, toDateOnlyString } from "./dateTime";

describe("parseDateOnly", () => {
  it("returns null for empty input", () => {
    expect(parseDateOnly(null)).toBeNull();
    expect(parseDateOnly(undefined)).toBeNull();
    expect(parseDateOnly("")).toBeNull();
  });

  it("parses a plain YYYY-MM-DD as local midnight", () => {
    const d = parseDateOnly("2026-06-30");
    expect(d).not.toBeNull();
    expect([d!.getFullYear(), d!.getMonth(), d!.getDate()]).toEqual([2026, 5, 30]);
    expect([d!.getHours(), d!.getMinutes(), d!.getSeconds()]).toEqual([0, 0, 0]);
  });

  it("accepts the datetime forms the backend renders for DATE columns", () => {
    // dateOnlyToRFC3339 on the Go side emits "<date>T00:00:00Z"; the other
    // shapes cover MySQL-style and offset variants a future caller might pass.
    for (const s of [
      "2026-06-30T00:00:00Z",
      "2026-06-30T00:00:00.000Z",
      "2026-06-30 00:00:00",
      "2026-06-30T12:30",
      "2026-06-30T05:30:00+05:30",
      "2026-06-30T05:30:00+0530",
    ]) {
      const d = parseDateOnly(s);
      expect(d, s).not.toBeNull();
      expect([d!.getFullYear(), d!.getMonth(), d!.getDate()], s).toEqual([2026, 5, 30]);
    }
  });

  it("takes the literal calendar date and discards the time/offset", () => {
    // Deliberate: for a DATE column this is correct — "2026-06-30" is June 30
    // no matter the viewer's timezone. It is WRONG for a real timestamp: pass
    // a genuine created_at and you get its UTC-literal day, not the local one.
    // Callers that need instant semantics must use parseBackendTimestamp.
    const d = parseDateOnly("2026-06-30T23:00:00Z");
    expect([d!.getFullYear(), d!.getMonth(), d!.getDate()]).toEqual([2026, 5, 30]);
  });

  it("returns null for a date with malformed trailing text", () => {
    for (const s of [
      "2026-06-30Tnot-a-date",
      "2026-06-30 garbage",
      "2026-06-30T",
      "2026-06-30xyz",
      "2026-06-30T99",
      "2026/06/30",
      "not-a-date",
    ]) {
      expect(parseDateOnly(s), s).toBeNull();
    }
  });

  it("returns null for an impossible calendar date", () => {
    expect(parseDateOnly("2026-13-40")).toBeNull();
    expect(parseDateOnly("2026-02-30")).toBeNull();
  });

  it("round-trips with toDateOnlyString", () => {
    expect(toDateOnlyString(parseDateOnly("2026-06-30"))).toBe("2026-06-30");
    expect(toDateOnlyString(parseDateOnly("2026-06-30T00:00:00Z"))).toBe("2026-06-30");
  });
});

describe("toDateOnlyString", () => {
  it("returns undefined for null/undefined", () => {
    expect(toDateOnlyString(null)).toBeUndefined();
    expect(toDateOnlyString(undefined)).toBeUndefined();
  });

  it("formats a local Date as YYYY-MM-DD", () => {
    expect(toDateOnlyString(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
