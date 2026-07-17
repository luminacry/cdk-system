/** CPA / codex2api → sub2api 浏览器本地转换器。
 *
 * 复用原 Python converter.py 的核心逻辑，所有 OAuth token 不离开用户设备。
 */

import JSZip from "jszip"

const MAX_INPUT_SIZE = 10 * 1024 * 1024; // 10 MB

export type DetectedFormat = "JSON 数组" | "JSONL" | "JSON 对象";

export interface ParsedInput {
  accounts: Record<string, unknown>[];
  format: DetectedFormat;
}

export interface ConvertSummary {
  index: number;
  email?: string;
  account_id?: string;
}

export function detectAndParse(text: string): ParsedInput {
  const trimmed = text.trim();
  if (!trimmed) {
    throw new Error("输入内容为空");
  }
  if (new Blob([trimmed]).size > MAX_INPUT_SIZE) {
    throw new Error("输入内容超过 10MB 限制");
  }

  // 1. 尝试 JSON 数组 / 单个对象
  try {
    const data = JSON.parse(trimmed);
    if (Array.isArray(data)) {
      if (data.length === 0) throw new Error("账号数组为空");
      if (!data.every((item) => typeof item === "object" && item !== null)) {
        throw new Error("JSON 数组中的元素必须是对象");
      }
      return { accounts: data as Record<string, unknown>[], format: "JSON 数组" };
    }
    if (typeof data === "object" && data !== null) {
      return { accounts: [data as Record<string, unknown>], format: "JSON 对象" };
    }
    throw new Error("JSON 根必须是数组或对象");
  } catch (e) {
    if (e instanceof SyntaxError) {
      // fall through to JSONL
    } else {
      throw e;
    }
  }

  // 2. JSONL
  const accounts: Record<string, unknown>[] = [];
  const lines = trimmed.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    try {
      const obj = JSON.parse(line);
      if (typeof obj !== "object" || obj === null) {
        throw new Error(`第 ${i + 1} 行必须是 JSON 对象`);
      }
      accounts.push(obj as Record<string, unknown>);
    } catch (e) {
      if (e instanceof SyntaxError) {
        throw new Error(`第 ${i + 1} 行不是有效 JSON`);
      }
      throw e;
    }
  }
  if (accounts.length === 0) throw new Error("未解析到任何账号");
  return { accounts, format: "JSONL" };
}

function decodeJWT(token: string): Record<string, unknown> {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return {};
    const b64 = parts[1] + "=".repeat((4 - (parts[1].length % 4)) % 4);
    const json = atob(b64.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function parseExpired(expired: string | undefined): number | null {
  if (!expired) return null;
  try {
    const dt = new Date(expired);
    if (isNaN(dt.getTime())) return null;
    return Math.floor(dt.getTime() / 1000);
  } catch {
    return null;
  }
}

function safeFilename(name: string): string {
  return name.replace(/[^a-zA-Z0-9@._-]/g, "_");
}

function filenameTimestamp(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
}

export function buildSummary(accounts: Record<string, unknown>[]): ConvertSummary[] {
  return accounts.map((acc, idx) => ({
    index: idx + 1,
    email: typeof acc.email === "string" ? acc.email : undefined,
    account_id: typeof acc.account_id === "string" ? acc.account_id : undefined,
  }));
}

export interface Sub2ApiAccount {
  exported_at: string;
  proxies: unknown[];
  accounts: unknown[];
}

export function convertOne(cpa: Record<string, unknown>, exportedAt: string, index: number): Sub2ApiAccount {
  const email = String(cpa.email ?? "");
  const accessToken = String(cpa.access_token ?? "");
  const accountId = String(cpa.account_id ?? "");
  const planType = String(cpa.plan_type ?? "");
  const idToken = String(cpa.id_token ?? "");
  const refreshToken = String(cpa.refresh_token ?? "");
  const expired = String(cpa.expired ?? "");

  const jwt = accessToken ? decodeJWT(accessToken) : {};
  const openaiAuth = (jwt["https://api.openai.com/auth"] as Record<string, unknown>) ?? {};
  const openaiProfile = (jwt["https://api.openai.com/profile"] as Record<string, unknown>) ?? {};

  const chatgptUserId = String(openaiAuth.chatgpt_user_id ?? "");
  const clientId = String(jwt.client_id ?? "");
  const expiresAt = parseExpired(expired);

  const name = email || `account_${String(index).padStart(4, "0")}`;
  const mm = String(new Date().getMonth() + 1).padStart(2, "0");
  const dd = String(new Date().getDate()).padStart(2, "0");
  const hh = String(new Date().getHours()).padStart(2, "0");
  const mi = String(new Date().getMinutes()).padStart(2, "0");

  const account: Record<string, unknown> = {
    name: `${name}-${mm}-${dd}-${hh}:${mi}`,
    platform: "openai",
    type: "oauth",
    credentials: {
      access_token: accessToken,
      chatgpt_account_id: accountId,
      chatgpt_user_id: chatgptUserId,
      client_id: clientId,
      email: email || String(openaiProfile.email ?? ""),
      expires_at: expiresAt,
      id_token: idToken,
      organization_id: "",
      plan_type: planType || String(openaiAuth.chatgpt_plan_type ?? ""),
      refresh_token: refreshToken,
    },
    extra: { email: email || String(openaiProfile.email ?? ""), converted_from: "cpa" },
    concurrency: 10,
    priority: 1,
    rate_multiplier: 1,
    auto_pause_on_expired: false,
  };

  for (const [key, value] of Object.entries(cpa)) {
    if (key !== "access_token" && key !== "id_token" && key !== "refresh_token") {
      (account.extra as Record<string, unknown>)[key] = value;
    }
  }

  return { exported_at: exportedAt, proxies: [], accounts: [account] };
}

export function convertAll(accounts: Record<string, unknown>[]): Sub2ApiAccount[] {
  const exportedAt = new Date().toISOString();
  return accounts.map((acc, idx) => convertOne(acc, exportedAt, idx + 1));
}

export function buildMerged(accounts: Record<string, unknown>[]): Sub2ApiAccount {
  const converted = convertAll(accounts);
  const mergedAccounts = converted.map((item) => item.accounts[0]);
  return { exported_at: new Date().toISOString(), proxies: [], accounts: mergedAccounts };
}

export async function createSingleZip(accounts: Record<string, unknown>[]): Promise<{ blob: Blob; filename: string }> {
  const converted = convertAll(accounts);
  const total = converted.length;
  const timestamp = filenameTimestamp();
  const zip = new JSZip();
  for (let i = 0; i < converted.length; i++) {
    const acc = accounts[i];
    const email = typeof acc.email === "string" ? acc.email : "";
    const identifier = email ? safeFilename(email) : `account_${String(i + 1).padStart(4, "0")}`;
    zip.file(`${String(i + 1).padStart(4, "0")}_${identifier}.json`, JSON.stringify(converted[i], null, 2));
  }
  const blob = await zip.generateAsync({ type: "blob", compression: "DEFLATE" });
  return { blob, filename: `04-sub_${total}_${timestamp}.zip` };
}

export async function createMergedZip(accounts: Record<string, unknown>[]): Promise<{ blob: Blob; filename: string }> {
  const total = accounts.length;
  const timestamp = filenameTimestamp();
  const zip = new JSZip();
  zip.file(`sub2api_merged_${total}_${timestamp}.json`, JSON.stringify(buildMerged(accounts), null, 2));
  const blob = await zip.generateAsync({ type: "blob", compression: "DEFLATE" });
  return { blob, filename: `04-sub_merged_${total}_${timestamp}.zip` };
}

export function createMergedJson(accounts: Record<string, unknown>[]): { blob: Blob; filename: string } {
  const total = accounts.length;
  const timestamp = filenameTimestamp();
  const blob = new Blob([JSON.stringify(buildMerged(accounts), null, 2)], { type: "application/json" });
  return { blob, filename: `sub2api_merged_${total}_${timestamp}.json` };
}
