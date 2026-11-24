# CI on CircleCI

The pipeline lives in `.circleci/config.yml` and mirrors the GitHub Actions workflow in `.github/workflows/ci.yml`:

| GitHub Actions step | CircleCI step |
| --- | --- |
| `actions/checkout` | `checkout` |
| `setup-go` 1.25 | primary image `cimg/go:1.25` |
| postgres service `pgvector/pgvector:pg16` | secondary image, same image and env |
| install gitleaks 8.24.3 | same download and install |
| install setuptools 70.3.0 and semgrep 1.110.0 | same pip install |
| pnpm 11 on node 22 | nvm installs node 22, npm installs pnpm 11 |
| `make verify` | same command |
| `go test ./gateway/memorybench` | same command |
| `make -C desktop verify` | same command |
| `make smoke` | same command |

## Cutting over

1. Connect the repository in CircleCI and let it read `.circleci/config.yml`.
2. Turn on the project setting "Only build pull requests", so feature branches build only when a pull request is open. The workflow always builds `main`.
3. Move the Postgres DSN into project environment variables if the database ever leaves localhost. It is inline for now, matching the GitHub workflow.
4. After a green run on `main` and on one pull request, turn off the GitHub workflow by deleting `.github/workflows/ci.yml` or disabling it in repository settings.

## What is verified here

The job commands were run in the same container image with the same Postgres service, so the steps are known to work. CircleCI's own scheduling, caching behaviour and image pull speed can only be confirmed by a real CircleCI run, which needs the repository connected to an account.
