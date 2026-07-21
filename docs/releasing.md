# Releasing RLViz

RLViz releases are built from versioned Git tags by
`.github/workflows/release.yml`. The workflow runs the full checks, builds four
native archives with GoReleaser, publishes SHA-256 checksums, creates GitHub
artifact attestations, and attaches a generated Homebrew formula.

## One-time setup

### Homebrew

The public `TheSnakeFang/homebrew-tap` repository polls the latest RLViz
release hourly and commits an updated attached formula with its repository-local
`GITHUB_TOKEN`. No cross-repository token is required. A maintainer can also run
the tap's `sync rlviz` workflow manually after a release.

### npm

The npm package is an optional installer for the same native archives. Its
trusted publisher is configured for `.github/workflows/release.yml`, and the
repository variable `NPM_PUBLISH_ENABLED` enables the job. Recovery and
re-bootstrap steps live in
[`packages/npm/README.md`](../packages/npm/README.md).

### Documentation

The documentation site is hosted on Vercel at `rlviz.dev`, not GitHub Pages.
Generate it locally and deploy the generated directory from the repository
root:

```bash
make site
vercel deploy site/dist
```

The generated `CNAME` file is harmless static output; Vercel project and domain
configuration remain the deployment source of truth.

Pushes to `main` deploy both static surfaces through
`.github/workflows/deploy.yml`. Configure these GitHub Actions settings before
enabling deployment:

| Kind | Name | Value |
| --- | --- | --- |
| Secret | `VERCEL_TOKEN` | Vercel access token authorized for the team and both projects |
| Variable | `VERCEL_ORG_ID` | `team_Ne0q1qtstphdUEi4nvQYFluC` |
| Variable | `VERCEL_PROJECT_ID_DOCS` | Project ID for `rlviz` (`rlviz.dev`) |
| Variable | `VERCEL_PROJECT_ID_APP` | Project ID for `rlviz-app` (`app.rlviz.dev`) |

The workflow fails before building if any setting is empty. It reads the Go
toolchain version from `go.mod`, builds `site/dist` and `webapp/dist` (including
the Go WebAssembly core), and deploys each directory to its own project. Forks
emit a notice and do not deploy.

## Publish

1. Choose the release version and confirm it does not already exist locally or
   on GitHub:

   ```bash
   VERSION=0.2.0
   if git rev-parse "v${VERSION}" >/dev/null 2>&1; then
     echo "local tag v${VERSION} already exists" >&2
     exit 1
   fi
   if git ls-remote --exit-code --tags origin "refs/tags/v${VERSION}" >/dev/null 2>&1; then
     echo "remote tag v${VERSION} already exists" >&2
     exit 1
   fi
   ```

2. Run the local release gates from a clean `main` branch at the same commit as
   `origin/main`:

   ```bash
   test "$(git branch --show-current)" = main
   test -z "$(git status --porcelain)"
   git fetch origin main --tags
   test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
   make check
   go test -race ./...
   goreleaser release --snapshot --clean
   ```

3. Create an annotated tag at the audited commit, then push that exact tag:

   ```bash
   git tag -a "v${VERSION}" -m "rlviz v${VERSION}"
   test "$(git rev-list -n 1 "v${VERSION}")" = "$(git rev-parse HEAD)"
   git push origin "v${VERSION}"
   ```

   The release workflow derives the npm package version from the tag before
   publishing, so `packages/npm/package.json` does not need a release-only
   commit.

4. Verify the GitHub release contains four archives, `checksums.txt`, and
   `rlviz.rb`, and that the attestation step passed.
5. On a clean machine, exercise one native archive, the curl installer, the
   Homebrew formula when enabled, and the npm package when enabled.

Do not reuse or move a published tag. Fix a broken release with a new patch
version. Checksums, package versions, and archive URLs are immutable release
contracts.
