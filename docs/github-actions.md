# GitHub Actions

Timoni can be used in GitHub workflows to perform actions
such as build, test and push modules to container registries.

## Usage

To run Timoni commands on GitHub Linux runners,
add the following steps to your GitHub workflow:

```yaml
steps:
  - name: Setup Timoni
    uses: stefanprodan/timoni/actions/setup@main
    with:
      version: latest # latest or exact version e.g. 0.0.2
      arch: amd64 # can be amd64 or arm64
      token: ${{ secrets.GITHUB_TOKEN }}
  - name: Run Timoni
    run: timoni version
```

## Examples

### Push to GitHub Container Registry

Example workflow for linting, testing and pushing a module to GitHub Container Registry:

```yaml
name: Release module
on:
  push:
    tag: ['*'] # semver format

permissions:
  contents: read # needed for checkout
  packages: write # needed for GHCR access

jobs:
  push:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Setup CUE
        uses: cue-lang/setup-cue@main
      - name: Setup Timoni
        uses: stefanprodan/timoni/actions/setup@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - name: Lint
        run: |
          timoni mod lint ./modules/my-module
      - name: Test instance build
        run: |
          timoni build -n testing test ./modules/my-module
      - name: Push
        run: |
          timoni mod push ./modules/my-module \
            oci://ghcr.io/${{ github.repository_owner }}/modules/my-module \
          	--version ${{ github.ref_name }} \
            --creds ${{ github.actor }}:${{ secrets.GITHUB_TOKEN }}
```

### Push to Docker Hub

Example workflow for using `docker login` to authenticate to Docker Hub:

```yaml
name: Release module
on:
  push:
    tag: ['*'] # semver format

permissions:
  contents: read # needed for checkout

jobs:
  push:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Setup Timoni
        uses: stefanprodan/timoni/actions/setup@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - name: Login to Docker Hub
        uses: docker/login-action@v2
        with:
          registry: docker.io
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}
      - name: Push
        run: |
          timoni mod push ./modules/my-module \
            oci://docker.io/my-org/my-module \
            --version ${{ github.ref_name }}
      - name: Pull
        run: |
          mkdir -p /tmp/my-module
          timoni mod pull oci://docker.io/my-org/my-module \
            --version ${{ github.ref_name }} \
            --output /tmp/my-module
```

Note that [docker/login-action](https://github.com/docker/login-action)
can be used to authenticate to any private registry including ACR, ECR, GCR.
