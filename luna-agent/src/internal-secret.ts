import { hkdfSync } from "node:crypto"

const salt = "luna-devops/ai-internal/v1"

function derive(secret: string, purpose: string): Buffer {
  return Buffer.from(hkdfSync(
    "sha256",
    Buffer.from(secret.trim()),
    Buffer.from(salt),
    Buffer.from(`luna-devops/ai/${purpose}/v1`),
    32,
  ))
}

export interface InternalKeys {
  serviceToken: string
  actorSigningKey: string
  callbackServiceToken: string
  runActorGrantSigningKey: string
  delegationTokenSigningKey: string
  runGrantEncryptionKey: Buffer
  toolArgumentsEncryptionKey: Buffer
}

export function deriveInternalKeys(secret: string): InternalKeys {
  if (Buffer.byteLength(secret.trim()) < 32) {
    throw new Error("AI_INTERNAL_SECRET must contain at least 32 bytes")
  }
  const deriveText = (purpose: string) => derive(secret, purpose).toString("base64url")
  return {
    serviceToken: deriveText("api-service-token"),
    actorSigningKey: deriveText("actor-context-signing-key"),
    callbackServiceToken: deriveText("agent-callback-service-token"),
    runActorGrantSigningKey: deriveText("run-actor-grant-signing-key"),
    delegationTokenSigningKey: deriveText("delegation-token-signing-key"),
    runGrantEncryptionKey: derive(secret, "run-grant-encryption-key"),
    toolArgumentsEncryptionKey: derive(secret, "tool-arguments-encryption-key"),
  }
}
