# Design

ExternalDNS's sources of DNS records live in package [source](https://github.com/kubernetes-sigs/external-dns/tree/master/source).
They implement the `Source` interface that has a single method `Endpoints` which returns the represented source's objects converted to `Endpoints`. Endpoints are just a tuple of DNS name and target where target can be an IP or another hostname.

For example, the `ServiceSource` returns all Services converted to `Endpoints` where the hostname is the value of the `external-dns.alpha.kubernetes.io/hostname` annotation and the target is the IP of the load balancer or the target is the IP of the service ClusterIP.

This list of endpoints is passed to the [Plan](https://github.com/kubernetes-sigs/external-dns/tree/master/plan) which determines the difference between the current DNS records and the desired list of `Endpoints`.

Once the difference has been figured out the list of intended changes is passed to a `Registry` which live in the [registry](https://github.com/kubernetes-sigs/external-dns/tree/master/registry) package.
The registry is a wrapper and access point to DNS provider. Registry implements the ownership concept by marking owned records and filtering out records not owned by ExternalDNS before passing them to DNS provider.

The [provider](https://github.com/kubernetes-sigs/external-dns/tree/master/provider) is the adapter to the DNS provider, e.g. Google Cloud DNS.
It implements two methods: `ApplyChanges` to apply a set of changes filtered by `Registry` and `Records` to retrieve the current list of records from the DNS provider.

The orchestration between the different components is controlled by the [controller](https://github.com/kubernetes-sigs/external-dns/tree/master/controller).

You can pick which `Source` and `Provider` to use at runtime via the `--source` and `--provider` flags, respectively.

## Adding a DNS Provider

A typical way to start on, e.g. a CoreDNS provider, would be to add a `coredns.go` to the providers package and implement the interface methods. Then you would have to register your provider under a name in `main.go`, e.g. `coredns`, and would be able to trigger it's functions via setting `--provider=coredns`.

Note, how your provider doesn't need to know anything about where the DNS records come from, nor does it have to figure out the difference between the current and the desired state, it merely executes the actions calculated by the plan.

## Running GitHub Actions locally

You can also extend the CI workflow which is currently implemented as GitHub Action within the [workflow](https://github.com/kubernetes-sigs/external-dns/tree/HEAD/.github/workflows) folder.
In order to test your changes before committing you can leverage [act](https://github.com/nektos/act) to run the GitHub Action locally.

Follow the installation instructions in the nektos/act [README.md](https://github.com/nektos/act/blob/master/README.md).
Afterwards just run `act` within the root folder of the project.

For further usage of `act` refer to its documentation.
