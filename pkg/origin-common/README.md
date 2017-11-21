## Packages based on the code from the OpenShift Origin repository

### clientcmd

The clientcmd package is a redacted copy of [github.com/openshift/origin/pkg/cmd/util/clientcmd](https://godoc.org/github.com/openshift/origin/pkg/cmd/util/clientcmd).

The code is almost untouched, but there are some differences:

  * some depedencies was merged into this package (getEnv, Addr, recommendedHomeFile, etc.),
  * it doesn't support migrations for `KUBECONFIG` (i.e. the old default is ignored, which is `.kube/.config`),
  * it uses the field `openshift.kubeconfig` from our config instead of the `--config` flag.

### crypto

The crypto package is a reduced copy of [github.com/openshift/origin/pkg/cmd/server/crypto](https://godoc.org/github.com/openshift/origin/pkg/cmd/server/crypto).

We keep only functions that are required by the image registry.
