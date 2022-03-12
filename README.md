# kuver

Kubernetes static old version detector

# usage

kuver finds specified directory recursively. And fail if older version exists.

```bash
go run main.go --dir ./path
```

# supported

- [x] HelmRelease(FluxCD2)
- [ ] kubernetes manifests with `image:` ( in working )
