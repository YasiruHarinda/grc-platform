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

/**
 * Content types safe to open inline in a new tab: none of them can carry
 * script that executes in this app's origin. Everything else is forced to
 * download instead. This matters because the backend's Content-Disposition:
 * attachment header has no effect here — the file is fetched via JS and
 * opened as a blob: URL, which the browser renders by content type
 * regardless of that header.
 */
const SAFE_INLINE_CONTENT_TYPES = new Set([
  "application/pdf",
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  "text/plain",
  "text/csv",
  "application/msword",
  "application/vnd.ms-excel",
  "application/vnd.ms-powerpoint",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
]);

/** Opens a fetched file inline when safe to render, otherwise forces a download. */
export function viewOrDownloadBlob(blob: Blob, fileName: string): void {
  const contentType = blob.type.split(";")[0].trim().toLowerCase();
  if (SAFE_INLINE_CONTENT_TYPES.has(contentType)) {
    const objectUrl = URL.createObjectURL(blob);
    window.open(objectUrl, "_blank", "noopener,noreferrer");
    setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
  } else {
    downloadBlob(blob, fileName);
  }
}

/**
 * Always saves the fetched file to disk, regardless of content type — for an
 * explicit "Download" action alongside "View" (which opens PDFs/images/Office
 * docs inline instead, per SAFE_INLINE_CONTENT_TYPES above).
 */
export function downloadBlob(blob: Blob, fileName: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = objectUrl;
  link.download = fileName;
  link.click();
  setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
}
