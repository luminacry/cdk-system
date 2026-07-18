import JSZip from "jszip"

import type { RedeemBatchItem } from "./types"

function safeFilename(value: string): string {
  const safe = value.replace(/[^a-zA-Z0-9._-]/g, "_")
  return safe || "redeemed-item"
}

function timestamp(): string {
  const now = new Date()
  const pad = (value: number) => String(value).padStart(2, "0")
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
}

export async function createRedeemArchive(results: RedeemBatchItem[]): Promise<{
  blob: Blob
  filename: string
  count: number
}> {
  const successful = results.filter((item) => item.ok && item.redemption)
  if (successful.length === 0) {
    throw new Error("没有可打包的成功兑换结果")
  }

  const zip = new JSZip()
  successful.forEach((item, index) => {
    const redemption = item.redemption!
    const number = String(index + 1).padStart(4, "0")
    const code = safeFilename(redemption.code || item.code)
    const hasCredential = redemption.credential !== undefined
    const filename = hasCredential
      ? `${number}_credential-${code}.json`
      : `${number}_redemption-${code}.json`
    const content = hasCredential ? redemption.credential : redemption
    zip.file(filename, JSON.stringify(content, null, 2) ?? "null")
  })

  const blob = await zip.generateAsync({ type: "blob", compression: "DEFLATE" })
  return {
    blob,
    filename: `redeemed-files_${successful.length}_${timestamp()}.zip`,
    count: successful.length,
  }
}
