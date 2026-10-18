/**
 * 日志下载工具：把查询结果序列化为 JSON 或按行分隔的原始日志文本，并用浏览器
 * CompressionStream 做 gzip 压缩。纯函数，抓取分页逻辑在 dataLogs 模型层。
 */

const ANSI_PATTERN = /\u001b\[[0-9;]*m/g;

export const stripAnsi = (value: string) => value.replace(ANSI_PATTERN, "");

function parseJsonObject(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  if (typeof value !== "string") {
    return null;
  }
  const text = value.trim();
  if (!text || (!text.startsWith("{") && !text.startsWith("["))) {
    const objectStart = text.indexOf("{");
    const objectEnd = text.lastIndexOf("}");
    if (objectStart < 0 || objectEnd <= objectStart) {
      return null;
    }
    return parseJsonObject(text.slice(objectStart, objectEnd + 1));
  }
  try {
    const parsed = JSON.parse(text) as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return null;
  }
  return null;
}

function parseJsonArray(value: unknown): unknown[] | null {
  if (Array.isArray(value)) {
    return value;
  }
  if (typeof value !== "string") {
    return null;
  }
  if (!value.trim().startsWith("[")) {
    return null;
  }
  try {
    const parsed = JSON.parse(value.trim()) as unknown;
    return Array.isArray(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function sanitizeLogJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value
      .map((item) => sanitizeLogJsonValue(item))
      .filter((item) => item !== undefined);
  }
  if (value && typeof value === "object") {
    return sanitizeLogJsonObject(value as Record<string, unknown>);
  }
  if (typeof value === "string") {
    const parsedJson = parseJsonObject(value);
    if (parsedJson) {
      return sanitizeLogJsonObject(parsedJson);
    }
    const parsedJsonArray = parseJsonArray(value);
    if (parsedJsonArray) {
      return parsedJsonArray
        .map((item) => sanitizeLogJsonValue(item))
        .filter((item) => item !== undefined);
    }
    return stripAnsi(value);
  }
  return value;
}

export function sanitizeLogJsonObject(row: Record<string, unknown>) {
  const result: Record<string, unknown> = {};
  Object.entries(row).forEach(([key, value]) => {
    const sanitizedValue = sanitizeLogJsonValue(value);
    if (sanitizedValue !== undefined) {
      result[key] = sanitizedValue;
    }
  });
  return result;
}

/** JSON 结构下载：每行日志序列化为对象，嵌套 JSON 字段解析成结构，去 ANSI */
export function formatDownloadedLogs(
  rows: Array<Record<string, unknown>>
): string {
  return JSON.stringify(rows.map((row) => sanitizeLogJsonObject(row)));
}

/** 原始日志下载：只取 _raw_log_，一条一行（行内换行压成空格），缺失的行跳过 */
export function formatRawLogsToLineDelimited(
  rows: Array<Record<string, unknown>>
): string {
  return rows
    .map((row) => {
      const raw = row["_raw_log_"];
      if (raw === undefined || raw === null || raw === "") {
        return "";
      }
      const text = typeof raw === "string" ? raw : JSON.stringify(raw);
      return stripAnsi(text).replace(/[\r\n]+/g, " ");
    })
    .filter((line) => line.length > 0)
    .join("\n");
}

export function downloadBlobFile(filename: string, blob: Blob) {
  const objectUrl = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = objectUrl;
  link.download = filename;
  link.rel = "noopener";
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 4000);
}

/** gzip 压缩；浏览器不支持 CompressionStream 时回退为未压缩文件 */
export async function compressToBlob(
  text: string,
  mime: string,
  extension: string
): Promise<{ blob: Blob; extension: string }> {
  const plainBlob = new Blob([text], { type: `${mime};charset=utf-8` });
  const CompressionStreamCtor = (window as any).CompressionStream;
  if (typeof CompressionStreamCtor !== "function") {
    return { blob: plainBlob, extension };
  }
  try {
    const compressed = await new Response(
      (plainBlob.stream() as any).pipeThrough(new CompressionStreamCtor("gzip"))
    ).blob();
    return {
      blob: new Blob([compressed], { type: "application/gzip" }),
      extension: `${extension}.gz`,
    };
  } catch {
    return { blob: plainBlob, extension };
  }
}
