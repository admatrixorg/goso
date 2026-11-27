#!/bin/sh
# Score the Minh An (demo) knowledge-relevance labels with Jev.
# Exits 0 and prints a skip line when GOSO_JEV_API_KEY is unset.
set -e
cd "$(dirname "$0")/.."
exec go run ./gateway/cmd/jev-eval
