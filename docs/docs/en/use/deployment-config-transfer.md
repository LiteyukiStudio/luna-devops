# Import and export deploy configs

A deploy config can be exported as JSON and imported into another app on the same platform. Import creates a new config; it never overwrites an existing config or starts a build or release.

## Export a config

1. Open the app's **Deployments** page.
2. Open the target deploy config's action menu and select **Export JSON**.
3. Store the downloaded file appropriately.

The file contains runtime specs, build settings, release policy, and non-secret configuration. It excludes account credentials, Secret values, source project-space resource IDs, live runtime state, and build or release history. It may still contain image addresses, environment variables, and config-file content, so handle it as internal configuration data.

## Import a config

1. Open the destination app's **Deployments** page and select **Import JSON**.
2. Select an exported JSON file.
3. Confirm the destination name, stage, and namespace.
4. Resolve every resource mapping reported by preflight and re-enter each Secret. When many candidates are available, search by name or type, sort, and move between pages instead of entering a resource ID manually.
5. When the status becomes **Ready to import**, select **Import config**.

The new config appears in the list after import. Luna DevOps does not automatically build or release it; review the result before starting either action.

## Recover from an unsupported stage

Public deployment stages are limited to `dev`, `test`, `staging`, and `prod`; `production` in a file is treated as `prod`. If an older file, or a file exported from an existing config, contains another stage such as `qa`, the first preflight reports an invalid stage and creates no deployment config, Secret, build environment, or volume mount.

Choose an allowed destination stage, then select **Run preflight again**. Commit is enabled only after preflight becomes **Ready to import**. Do not edit the summary or reuse a digest from an invalid preflight; import revalidates the file, overrides, and digest.

## Cross-project imports and empty apps

The file describes dependencies but does not copy them:

- A repository source must select a repository already bound to the destination app. If the app has none, bind one first and run preflight again.
- A source build must select a registry available to the destination project space with a push credential.
- Runtime clusters, build variable sets, runtime config sets, hooks, and project volumes must map to accessible existing resources in the destination project space.
- A project volume mapping creates only a mount relation; it does not copy a PVC or its data. Use [volume import and export](./volume-transfer) first when data must move.
- An image source can be imported into an empty app with no repository or build history when its image reference and other dependencies are valid.

Luna DevOps creates no partial config when a candidate is missing, ambiguous, forbidden, or incompatible. Prepare or adjust the destination resource, then run preflight again.

## Troubleshooting

### Why must I choose another stage?

A stage is unique within an app. The first import version does not overwrite or merge an existing deploy config, so select an unused stage.

### Why must I enter Secrets again?

The export records only the purpose and key of each Secret, never its value or storage reference. New values are encrypted during the confirmed import.

### Why is the app not running after import?

Import creates desired configuration only. It does not create a build run, release, or Kubernetes workload. Check the repository, image, and runtime configuration, then build and release explicitly.
