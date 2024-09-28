# Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add secrets https://udhos.github.io/secrets

Update files from repo:

    helm repo update

Search secrets:

    $ helm search repo secrets -l --version ">=0.0.0"
    NAME                    CHART VERSION	APP VERSION	DESCRIPTION
    secrets/secrets	0.3.0   0.3.0      	A Helm chart for Kubernetes
    secrets/secrets	0.2.0   0.2.0      	A Helm chart for Kubernetes
    secrets/secrets	0.1.0   0.1.0      	A Helm chart for Kubernetes

To install the charts:

    helm install my-secrets secrets/secrets
    #            ^          ^       ^
    #            |          |        \__________ chart
    #            |          |
    #            |           \__________________ repo
    #            |
    #             \_____________________________ release (chart instance installed in cluster)

To uninstall the charts:

    helm uninstall my-secrets

# Source

<https://github.com/udhos/secrets>