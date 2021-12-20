# CentOS 8 and derivatives

CentOS 8 / Oracle Linux 8 / AlmaLinux 8 / Rocky Linux 8 ship only with iptables-nft (ie without iptables-legacy similar to RHEL8)
The only tested configuration for now is using Calico CNI
You need to add `calico_iptables_backend: "NFT"` or `calico_iptables_backend: "Auto"` to your configuration.

If you have containers that are using iptables in the host network namespace (`hostNetwork=true`),
you need to ensure they are using iptables-nft.
An example how k8s do the autodetection can be found [in this PR](https://github.com/kubernetes/kubernetes/pull/82966)
