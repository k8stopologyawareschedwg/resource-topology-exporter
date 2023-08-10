# k8simported

This is copy-pasted code to minimize k8s deps, first and foremost against `k8s.io/kubernetes`
Please remove packages and replace with proper vendor (easier to track and work with) when
options are available.

- `cpuset`: cloned from `k8s.io/utils`@`3b25d923346b3814e0898684c97390092f31a61e` remove when switching to 1.28.z
