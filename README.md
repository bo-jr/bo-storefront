# bo-storefront

Source for the `storefront` service — the canary target.

Part of a three-cluster GitOps lab demonstrating progressive delivery gated on
**error-budget burn rate**. Spec: [`bo-platform/BUILD-PLAN.md`](https://github.com/bo-jr/bo-platform/blob/main/BUILD-PLAN.md) ·
Working rules: [`CLAUDE.md`](./CLAUDE.md)

`GET /checkout?sku=X&qty=N`, fans out to `catalog` and `pricing`. Source only; manifests are rendered by CI into [`bo-deploy`](https://github.com/bo-jr/bo-deploy).

## How a commit reaches dev

`.github/workflows/ci.yml` calls bo-platform's reusable
[`service-ci.yml`](https://github.com/bo-jr/bo-platform/blob/main/.github/workflows/service-ci.yml),
pinned by commit SHA.

- **Pull request:** `go vet` and `go test`, then the chart renders with a placeholder
  digest and the lab's Kyverno policies run against it. Nothing is published.
- **Push to `main`:**
  - `linux/amd64` and `linux/arm64` build on native runners and are smoke-tested.
  - The index is pushed to `ghcr.io/bo-jr/bo-storefront` by digest, with no tag.
  - It is signed with cosign (keyless) and gets SLSA build provenance.
  - CI opens or force-pushes the rolling `dev/storefront` PR in
    [`bo-deploy`](https://github.com/bo-jr/bo-deploy), which auto-merges once its
    required `validate` check passes. The run fails if that PR doesn't merge.

Check an image yourself:

```bash
cosign verify ghcr.io/bo-jr/bo-storefront@sha256:<digest> --certificate-identity-regexp '^https://github\.com/bo-jr/bo-platform/\.github/workflows/service-ci\.yml@[0-9a-f]{40}$' --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

```bash
gh attestation verify oci://ghcr.io/bo-jr/bo-storefront@sha256:<digest> --repo bo-jr/bo-storefront --signer-workflow bo-jr/bo-platform/.github/workflows/service-ci.yml --format json
```

The inner loop (`task sandbox:build` / `sandbox:deploy` in bo-platform) builds locally
and deploys to the unmanaged `sandbox` namespace instead; nothing there is promoted.
