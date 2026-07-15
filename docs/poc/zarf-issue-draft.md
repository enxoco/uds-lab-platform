---
name: Bug report
about: Create a report to help us improve
title: 'zarf init fails when injector payload ConfigMap disappears during cleanup'
labels: 'possible-bug 🐛'
assignees: ''
---

### Environment
Device and OS:
Ubuntu 24.04 amd64 VM, running on CachyOS host

App version:
Zarf v0.80.0, invoked through UDS CLI v0.33.0

Kubernetes distro being used:
K3s v1.36.2+k3s1

Other:
Internal registry mode / nodeport mode. UDS Core playground setup. The init
package is the signed init package bundled with the UDS CLI.

### Steps to reproduce
1. Start a Kubernetes cluster with a reachable API server.
2. Run `zarf init --confirm` using the default internal registry configuration.
3. During deployment of the `zarf-seed-registry` component, arrange for one of
   the `zarf-payload-*` ConfigMaps to be deleted after Zarf lists payload
   ConfigMaps but before Zarf deletes them.

The attached local POC demonstrates the Kubernetes behavior without requiring
Zarf internals:

`docs/poc/zarf-injector-cleanup-race.sh`

Public copy: https://gist.github.com/enxoco/dc04785a915a8322803b8e5c30fedc45

It creates a payload ConfigMap, records it from a list operation, deletes it,
then repeats the plain delete. The second delete returns `NotFound`; deleting
with `--ignore-not-found` succeeds.

### Expected result

Injector cleanup is idempotent. If another cleanup path has already deleted a
payload ConfigMap, `zarf init` should continue because the desired state has
already been reached.

### Actual Result

`zarf init` fails after the `zarf-seed-registry` Helm chart reports healthy:

```text
ERR failed to deploy package: unable to deploy component "zarf-seed-registry":
failed to delete injector resources: configmaps "zarf-payload-031" not found
```

The cluster can be left partially initialized even though the registry health
checks passed. In the affected setup, the surrounding task treats this as a
failed initialization and stops the platform build.

The relevant cleanup pattern in `src/pkg/cluster/injector.go` is:

1. Delete the labeled payload ConfigMaps with `DeleteCollection`.
2. List all ConfigMaps in the Zarf namespace for compatibility with older
   payload ConfigMaps that have no labels.
3. Delete each `zarf-payload-*` ConfigMap returned by that list.

The per-ConfigMap delete does not ignore `apierrors.IsNotFound`. A resource can
disappear between the list and delete calls, producing this failure.

### Visual Proof (screenshots, videos, text, etc)

Reproduction POC: https://gist.github.com/enxoco/dc04785a915a8322803b8e5c30fedc45

Full observed output:

```text
2026-07-14 08:28:05 INF starting deploy package=init
2026-07-14 08:28:05 INF deploying component name=zarf-injector
2026-07-14 08:28:09 INF creating Zarf injector resources
2026-07-14 08:28:09 INF adding archived binary configmaps of registry image to the cluster
2026-07-14 08:28:20 INF deploying component name=zarf-seed-registry
2026-07-14 08:28:20 INF processing Helm chart name=docker-registry version=1.0.1 source=Zarf-generated
2026-07-14 08:28:20 INF performing Helm install chart=docker-registry
2026-07-14 08:28:29 INF running health checks chart=docker-registry
2026-07-14 08:28:34 ERR failed to deploy package: unable to deploy component "zarf-seed-registry": failed to delete injector resources: configmaps "zarf-payload-031" not found
```

### Severity/Priority

Possible bug / medium priority.

This blocks `zarf init` and can break automated cluster provisioning. It does
not appear to corrupt the registry or indicate a registry health failure. The
failure is timing-dependent, so prevalence is currently unknown.

### Additional Context

#### How common is this likely to be?

Confirmed:

- This occurred once during one automated build.
- The failure occurred after the seed registry health check succeeded.
- The current cleanup implementation has a real list-then-delete race.
- The local POC reproduces the underlying Kubernetes `NotFound` behavior
  deterministically.

Not yet established:

- Whether the injector itself, another Zarf cleanup path, or Kubernetes is the
  competing deleter in normal `zarf init` runs.
- The failure rate across Kubernetes distributions, API-server load levels,
  and payload sizes.
- Whether the race is limited to legacy unlabeled payload ConfigMaps.

The issue likely requires the internal nodeport seed-registry path and a
concurrent or earlier cleanup of payload ConfigMaps. It should therefore be
intermittent rather than affecting every initialization, with higher exposure
under API-server load or when initialization is retried.

#### Likely fix effort

Low implementation effort:

- Treat `NotFound` as success for the per-object payload ConfigMap delete.
- Consider applying the same idempotent handling to the other injector cleanup
  deletes and `DeleteCollection` if needed.

Moderate validation effort:

- Add a unit test using a Kubernetes fake client or reactor that deletes an
  object between list and delete.
- Add or extend an init/injector integration test that exercises cleanup after
  seed-registry deployment.
- Verify behavior with labeled and legacy unlabeled payload ConfigMaps.

#### Workaround tradeoffs

Current downstream workaround: retry `zarf init` when the error specifically
matches this payload ConfigMap `NotFound` failure, cleaning leftover injector
resources before retrying.

Advantages:

- No Zarf fork required.
- Narrowly scoped to the known failure.
- Preserves failure behavior for unrelated init errors.

Disadvantages:

- Adds provisioning latency.
- May still fail after repeated attempts.
- Masks an upstream cleanup error and complicates automation.
- Does not improve Zarf for other users.

Preferred upstream fix: make injector cleanup idempotent by ignoring
`NotFound`, then cover the race with a regression test. This is simpler and
more reliable than requiring downstream retry logic.

#### Related work

The following historical injector PRs were reviewed but do not appear to fix
this specific list-then-delete race:

- https://github.com/zarf-dev/zarf/pull/2629
- https://github.com/zarf-dev/zarf/pull/510
- https://github.com/zarf-dev/zarf/pull/2956

No existing open issue or PR matching this failure was found during review.
