# Versioning and safe upgrade paths

tuppr upgrades to **exactly** the version you put in a `TalosUpgrade` - it does
not validate that the jump is a supported one. Choosing a safe target is your
responsibility.

## Talos's supported upgrade path

Talos supports upgrading through each **minor** version in sequence; skipping
minors is unsupported. See the upstream
[supported upgrade paths](https://www.talos.dev/latest/talos-guides/upgrading-talos/#supported-upgrade-paths).
For example, going from v1.0 to v1.2.4 means:

1. v1.0.x → latest patch of v1.0 (e.g. v1.0.6)
2. v1.0.6 → latest patch of v1.1 (e.g. v1.1.2)
3. v1.1.2 → v1.2.4

/// danger | tuppr does not enforce this
Set `spec.talos.version` to a version that is a valid next step from your
current one. tuppr will happily attempt whatever you specify.
///

## Let Renovate propose safe steps

[Renovate](https://docs.renovatebot.com/) can bump the version in your GitOps
repo one hop at a time if you separate major/minor from minor/patch PRs:

```json
{
  "packageRules": [
    {
      "matchDatasources": ["docker"],
      "matchPackageNames": ["ghcr.io/siderolabs/installer"],
      "separateMajorMinor": true,
      "separateMinorPatch": true
    }
  ]
}
```

- [`separateMajorMinor`](https://docs.renovatebot.com/configuration-options/#separatemajorminor)
  - separate PRs for major vs minor bumps.
- [`separateMinorPatch`](https://docs.renovatebot.com/configuration-options/#separateminorpatch)
  - separate PRs for minor vs patch bumps.

Renovate then finds and bumps the version through the `renovate:` comment on the
spec field, so you merge one incremental step at a time:

```yaml
spec:
  talos:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
    version: v1.13.7
```

The same pattern works for `KubernetesUpgrade` with
`depName=ghcr.io/siderolabs/kubelet`.
