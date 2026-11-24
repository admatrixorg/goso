# CI on CircleCI

CI for this repository is the CircleCI job `verify` in `.circleci/config.yml`.

It runs `make verify`, the memory-vi bench, `make -C desktop verify`, and `make smoke`, with a pgvector Postgres service. Tool versions match the former GitHub Actions workflow: Go 1.25, gitleaks 8.24.3, semgrep 1.110.0, Node 22.14.0, pnpm 11.

The GitHub Actions workflow was removed after CircleCI job 3 on main succeeded.
