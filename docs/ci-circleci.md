# CI on CircleCI

The pipeline in `.circleci/config.yml` mirrors `.github/workflows/ci.yml`.

The job `verify` runs on every push and every pull request. GitHub Actions stays on until a CircleCI run is green on main.

Cut over after a green CircleCI `verify` on main: disable `.github/workflows/ci.yml` in the repository settings, or delete that file.
