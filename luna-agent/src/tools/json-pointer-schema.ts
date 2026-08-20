import { z } from "zod"

const forbiddenPointerSegments = new Set(["__proto__", "prototype", "constructor"])
const canonicalArrayIndex = /^(?:0|[1-9]\d*)$/

export const safeJsonPointer = z.string()
  .regex(/^\/(?:[^/~]|~[01])+(?:\/(?:[^/~]|~[01])+)*$/)
  .max(240)
  .superRefine((value, context) => {
    const segments = value.slice(1).split("/").map(segment => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
    if (segments.some(segment => forbiddenPointerSegments.has(segment))) {
      context.addIssue({
        code: "custom",
        message: "JSON Pointer contains a forbidden object prototype segment.",
      })
    }
    if (segments.some(segment => canonicalArrayIndex.test(segment) && Number(segment) > 999)) {
      context.addIssue({
        code: "custom",
        message: "JSON Pointer array index exceeds the supported limit.",
      })
    }
  })
