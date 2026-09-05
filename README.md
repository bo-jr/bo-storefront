# bo-storefront

Source for the `storefront` service — the canary target.

Part of a three-cluster GitOps lab demonstrating progressive delivery gated on
**error-budget burn rate**. Spec: [`bo-platform/docs/BUILD-PLAN.md`](https://github.com/bo-jr/bo-platform/blob/main/docs/BUILD-PLAN.md) ·
Working rules: [`CLAUDE.md`](./CLAUDE.md)

`GET /checkout?sku=X&qty=N`, fans out to `catalog` and `pricing`. Source only; manifests are rendered by CI into [`bo-deploy`](https://github.com/bo-jr/bo-deploy).
