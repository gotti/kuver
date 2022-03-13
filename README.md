# kuver

Kubernetes static old version detector

# usage

kuver finds specified directory recursively. And fail if older versions exist.

```bash
go run main.go --dir ./path
```

# supported

- [x] HelmRelease(FluxCD2)
- [x] generic kubernetes manifests with `image:`

# features

kuver walks specified directory and read kubernetes manifests end with yaml or yml.

Older versions are detected by the following rules.

## HelmRelease

When kuver finds HelmRelease, it looks for the corresponding HelmRepository and looks at the repository URL.

Fetching repository URL, kuver finds version list and know whether using version is latest.

## generic kubernetes manifests with image:

When kuver finds `image:` in manifests, it checks image tag. If the tag is semver, kuver query the image registry for a list of tags and know wheter using version is latest.

Non-semver tags are ignored, such as `latest` or `slim`.
