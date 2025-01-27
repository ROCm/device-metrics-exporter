# Release Helm Chart Repository

```bash
# Build Helm Charts from a release branch
cd helm-charts; helm lint; cd ..; helm package helm-charts/
git checkout gh-pages
mv device-metrics-exporter-*.tgz ./charts/

# Update the index.yml
helm repo index . --url https://rocm.github.io/device-metrics-exporter

# Release
git add ./charts
git add index.yaml
git commit -m 'Release version XXX'

# deploy the new GitHub page
git push 
```
