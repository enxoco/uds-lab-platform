# Zarf Injector Cleanup Race POC

`zarf-injector-cleanup-race.sh` reproduces the failure seen during `zarf init`.
It lists a payload ConfigMap, deletes it between the list and delete calls, and
then repeats the plain delete used by Zarf. Kubernetes returns `NotFound` even
though the desired end state has already been reached.

Run against a disposable Kubernetes namespace:

```bash
sh docs/poc/zarf-injector-cleanup-race.sh
```

The idempotent form, `kubectl delete --ignore-not-found`, succeeds for the same
state. Zarf should treat `NotFound` as success while deleting injector cleanup
resources.
