# clusterfuzz

Run afl in parallel on a Kubernetes cluster, against targets of your choice.

## Description

clusterfuzz attempts to follow [the advice on how to run multiple afl instances](https://aflplus.plus/docs/fuzzing_in_depth/#3-fuzzing-the-target) to offer highly scalable fuzzing with minimal work required.

## Getting Started

### Prerequisites

- go version v1.25+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster, ideally 1.35+ for `ImageVolume` support.

### To Deploy on the cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/clusterfuzz:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/clusterfuzz:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
> privileges or be logged in as admin.

**Create a target**
My testing is all done with Rust, using [`cargo-afl`](https://crates.io/crates/cargo-afl). Build an executable with `cargo afl build`, and ensure its architecture matches the Kubernetes cluster where you expect to run the fuzzing. There are currently two ways of supplying those binaries to clusterfuzz:

- Upload them to an S3-compatible API (minio, S3, ceph+rados, etc), and use an `S3Target`:
  ```yaml
  apiVersion: fuzz.will.murnane.family/v1alpha1
  kind: S3Target
  metadata:
    name: s3target-sample
  spec:
    bucket: somebucket
    endpoint: https://minio-instance.somewhere # Optional, assumes S3 by default
    region: us-east-1 # Optional, consult your S3 provider but `us-east-1` is probably a good guess.
    accessKey: asdf # Optional, if credentials will be provided through some other mechanism (PIA, node role, mTLS, etc)
    secretKey: sdfg # Ditto
    binaries:
      amd64: /someprefix/myapp-x86-linux-gnu-unknown
      amd64_asan: /someprefix/myapp-x86-linux-gnu-unknown-asan
      arm64: /someprefix/myapp-arm64-linux-gnu-unknown
    # Size limit for downloading the binaries, must be larger than any of the binaries in the bucket.
    sizeLimit: 100Mi
  ```
- Create a container image, and use an `ImageTarget`:
  ```yaml
  apiVersion: fuzz.will.murnane.family/v1alpha1
  kind: ImageTarget
  metadata:
    name: imagetarget-sample
  spec:
    image: some.domain/my-target:v1.23
  ```

Now, create one or more `FuzzJob` objects which reference the name and kind of the Target, in the same namespace as the Targets:

```yaml
apiVersion: fuzz.will.murnane.family/v1alpha1
kind: FuzzJob
metadata:
  name: fuzzjob-sample
spec:
  cores: 24
  targetRef:
    name: s3target-sample
    kind: S3Target # or ImageTarget, if that's how you prefer to do it.
```

Adjust the number of cores to the size of cluster you have available.

### Runtime behavior

When the controller processes your `FuzzJob`, it will create a StatefulSet to execute the fuzzing process. Each pod will run 8 or 16 fuzzing containers, plus an extra containers to aggregate the statistics from the containers. These statistics will be put on the Pods as annotations. Then, the controller periodically collects the statistics from all the Pods that are running and aggregates them into the status of the FuzzJobs that created them. For example, here is the output of `kubectl get -o yaml fuzzjob fuzzjob-sample` after it has been running for a while:

```yaml
apiVersion: fuzz.will.murnane.family/v1alpha1
kind: FuzzJob
metadata:
  name: fuzzjob-sample
  namespace: default
  # ...
spec:
  # as before
status:
  progress:
    fuzz.will.murnane.family/corpus_count: "72781"
    fuzz.will.murnane.family/corpus_favored: "1116"
    fuzz.will.murnane.family/corpus_found: "30766"
    fuzz.will.murnane.family/corpus_imported: "41897"
    fuzz.will.murnane.family/corpus_variable: "13253"
    fuzz.will.murnane.family/cycles_done: "84730"
    fuzz.will.murnane.family/cycles_wo_finds: "4079"
    fuzz.will.murnane.family/execs_done: "21010970272"
    fuzz.will.murnane.family/execs_per_sec: "138319"
    fuzz.will.murnane.family/execs_ps_last_min: "133391"
    fuzz.will.murnane.family/fuzz_time: "3894092"
    fuzz.will.murnane.family/last_update: "[container 1]: 2025-12-28T18:45:46Z"
    fuzz.will.murnane.family/saved_crashes: "0"
    fuzz.will.murnane.family/saved_hangs: "0"
    fuzz.will.murnane.family/stats-timestamp: "[container 1]: 2025-12-28T18:45:48Z"
```

Watching this output provides a convenient summary: 21 billion executions done, no crashes or hangs, has updated recently so things are still churning away.

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/clusterfuzz:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/clusterfuzz/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
   can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
