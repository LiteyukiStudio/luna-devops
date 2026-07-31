import { initializeTelemetry, shutdownTelemetry, stableErrorCode, telemetryLog } from "./telemetry.js"

initializeTelemetry()

try {
  const { startAgent } = await import("./bootstrap.js")
  await startAgent()
}
catch (error) {
  telemetryLog("agent.start_failed", "error", {
    "error.type": error instanceof Error ? error.name : "UnknownError",
    "error.code": stableErrorCode(error),
  })
  await shutdownTelemetry()
  throw error
}
