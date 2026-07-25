import {
  existsSync,
  readdirSync,
  readFileSync,
  statSync,
} from "node:fs";
import { basename, join, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  requiredArgument,
  sha256,
} from "./lib.mjs";

function githubHeaders(token, contentType) {
  return {
    Accept: "application/vnd.github+json",
    Authorization: `Bearer ${token}`,
    "X-GitHub-Api-Version": "2022-11-28",
    ...(contentType ? { "Content-Type": contentType } : {}),
  };
}

async function githubJson(url, token, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      ...githubHeaders(token, "application/json"),
      ...options.headers,
    },
  });
  if (!response.ok) {
    const body = await response.text();
    const error = new Error(`GitHub API ${response.status}: ${body}`);
    error.status = response.status;
    throw error;
  }
  return response.json();
}

async function existingRelease(repository, tag, token) {
  try {
    return await githubJson(
      `https://api.github.com/repos/${repository}/releases/tags/${encodeURIComponent(tag)}`,
      token,
    );
  } catch (error) {
    if (error.status === 404) {
      return null;
    }
    throw error;
  }
}

async function remoteAssetSha256(asset, token) {
  if (asset.digest?.startsWith("sha256:")) {
    return asset.digest.slice("sha256:".length);
  }
  const response = await fetch(asset.url, {
    headers: {
      ...githubHeaders(token),
      Accept: "application/octet-stream",
    },
  });
  if (!response.ok) {
    throw new Error(`Unable to download existing asset ${asset.name}`);
  }
  const buffer = Buffer.from(await response.arrayBuffer());
  const { createHash } = await import("node:crypto");
  return createHash("sha256").update(buffer).digest("hex");
}

async function uploadAsset(release, path, token) {
  const name = basename(path);
  const uploadUrl = release.upload_url.replace(/\{.*$/, "");
  const response = await fetch(
    `${uploadUrl}?name=${encodeURIComponent(name)}`,
    {
      method: "POST",
      headers: githubHeaders(token, "application/octet-stream"),
      body: readFileSync(path),
    },
  );
  if (!response.ok) {
    throw new Error(
      `Unable to upload ${name}: ${response.status} ${await response.text()}`,
    );
  }
}

export async function publishGithubRelease({
  repository,
  tag,
  name,
  directory,
  prerelease,
  token,
}) {
  const root = resolve(directory);
  const notesPath = join(root, "RELEASE_NOTES.md");
  if (!existsSync(notesPath)) {
    throw new Error(`Release notes are missing: ${notesPath}`);
  }

  let release = await existingRelease(repository, tag, token);
  if (!release) {
    release = await githubJson(
      `https://api.github.com/repos/${repository}/releases`,
      token,
      {
        method: "POST",
        body: JSON.stringify({
          tag_name: tag,
          name,
          body: readFileSync(notesPath, "utf8"),
          draft: false,
          prerelease,
          generate_release_notes: true,
        }),
      },
    );
  } else if (Boolean(release.prerelease) !== prerelease) {
    throw new Error(
      `Existing release ${tag} has a different prerelease setting`,
    );
  }

  const files = readdirSync(root)
    .map(name => join(root, name))
    .filter(path => statSync(path).isFile())
    .filter(path => basename(path) !== "RELEASE_NOTES.md");
  const assets = new Map(release.assets.map(asset => [asset.name, asset]));

  for (const path of files) {
    const name = basename(path);
    const existing = assets.get(name);
    if (existing) {
      const remoteHash = await remoteAssetSha256(existing, token);
      const localHash = sha256(path);
      if (remoteHash !== localHash) {
        throw new Error(
          `Existing GitHub Release asset ${name} has different content`,
        );
      }
      process.stdout.write(`${name} already exists with matching SHA-256.\n`);
      continue;
    }
    await uploadAsset(release, path, token);
    process.stdout.write(`Uploaded ${name}.\n`);
  }
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const token = process.env.GITHUB_TOKEN;
  if (!token) {
    throw new Error("GITHUB_TOKEN is required");
  }
  await publishGithubRelease({
    repository: requiredArgument(args, "repository"),
    tag: requiredArgument(args, "tag"),
    name: requiredArgument(args, "name"),
    directory: requiredArgument(args, "directory"),
    prerelease: args.get("prerelease") === "true",
    token,
  });
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
