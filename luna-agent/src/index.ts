import { errorDiagnostic, initializeTelemetry, shutdownTelemetry, telemetryLog } from "./telemetry.js"

try {
  initializeTelemetry()
  const { startAgent } = await import("./bootstrap.js")
  await startAgent()
}
catch (error) {
  telemetryLog("agent.start_failed", "error", {
    "operation": "agent.startup",
    "outcome": "failed",
    ...errorDiagnostic(error, "agent.startup.failed", "verify Agent configuration and PostgreSQL/Luna API connectivity"),
  }, "Agent startup failed")
  await shutdownTelemetry().catch(shutdownError => telemetryLog("agent.telemetry.shutdown_failed", "warn", {
    "operation": "agent.telemetry.shutdown",
    "outcome": "failed",
    ...errorDiagnostic(shutdownError, "telemetry.shutdown.failed"),
  }))
  process.exitCode = 1
}
