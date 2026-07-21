# rlviz

Install the native RLViz CLI through npm:

```bash
npm install --global rlviz
rlviz open ./trajectory.ndjson
```

The package downloads the matching macOS or Linux release archive, verifies its
published SHA-256 checksum, and installs the native `rlviz` binary. Normal
viewing is local and makes no outbound network requests.

The source, native archives, and other installation options are at
<https://github.com/TheSnakeFang/rlviz>.

## Publishing setup

Tag releases publish through npm trusted publishing (OIDC) with provenance.
The `rlviz` package is linked to `TheSnakeFang/rlviz` and the repository
variable `NPM_PUBLISH_ENABLED` is enabled.

The workflow deliberately does not accept a long-lived `NPM_TOKEN`. If the
package or trusted publisher ever needs to be recreated:

1. With `NPM_PUBLISH_ENABLED` still unset, create the matching GitHub release
   first (for example `v0.1.0`), then claim the package using an npm account with
   2FA:

   ```sh
   cd packages/npm
   npm login
   npm publish --access public --provenance=false
   ```

   The one-time provenance override is necessary because trusted publishing
   cannot be attached until the package exists. Subsequent CI publishes always
   require provenance.
2. In the package's npm settings, add a GitHub Actions trusted publisher for
   organization `TheSnakeFang`, repository `rlviz`, workflow `release.yml`,
   and allow `npm publish`. With npm 11.5.1+, the equivalent authenticated CLI
   command is:

   ```sh
   npm trust github rlviz --repo TheSnakeFang/rlviz --file release.yml --allow-publish
   ```
3. Prefer "Require two-factor authentication and disallow tokens" under npm
   publishing access after OIDC is working.
4. Set the GitHub repository variable `NPM_PUBLISH_ENABLED=true`.

The release job derives the npm version from the `v*` Git tag, tests and
dry-runs the package, then publishes only after the matching native archives
have succeeded. Trusted publishing requires npm 11.5.1+ and a GitHub-hosted
runner; the workflow uses Node 24 and `id-token: write`.
