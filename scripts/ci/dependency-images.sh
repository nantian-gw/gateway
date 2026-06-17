#!/usr/bin/env bash

# Last known-good dependency images for gateway kind validation.
# Update these intentionally after a dependency image has passed gateway E2E.
DEFAULT_DATAPLANE_IMAGE="ghcr.io/nantian-gw/dataplane@sha256:bacc962711a95fd8fa75e3f9206319a42490b6eaf94257d57fc48f998444ea0e"
DEFAULT_DASHBOARD_IMAGE="ghcr.io/nantian-gw/dashboard@sha256:74f4c0f4afbf3f8c0ec26110a31d2327ca45d97d0ebd0ce73a765b051d6c208a"
