# One artifact: a static binary in a distroless image.
#
# No runtime, no package manager, no shell. The image is what doc 17 promises
# and what every platform consumes -- Dokploy, Coolify, Kamal, Fly, Railway, ECS
# and Kubernetes all take an OCI image and need nothing else from us.

FROM golang:1.25-alpine AS build

# git for the version stamp, and nothing else. There is no node, no python and
# no build toolchain here, which is the whole point of RULE 13.
#
# No gcompat either, and that is worth a line: the standalone tailwindcss is
# published in a glibc build and a musl one, and `aru view:build` downloads the
# one this system can load. Alpine's glibc shim was the other answer, and it
# would have been a package in every project's Dockerfile instead of a decision
# in one place.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Dependencies first, so a code change does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

# CGO off is what makes the binary static and what lets distroless run it. The
# SQLite driver is pure Go for exactly this reason.
#
# -trimpath keeps the build machine's paths out of the binary, so a stack trace
# from production does not name somebody's home directory.

# The views, compiled in the image rather than copied in.
#
# They are build output and are not in the repository (see .gitignore), so the
# binary would not link without this. Installing the CLI here costs a layer and
# buys the property that the image is built from sources only -- nothing in it
# came from somebody's laptop.
# Pinned, not @latest.
#
# An image built from @latest depends on when it was built, which is the one
# property an image is supposed to not have: two builds of the same commit
# produce different binaries and nobody can say which is deployed.
#
# It is also what @latest costs in practice -- the module proxy caches that
# endpoint separately from the version list, so a fresh release is in the list
# and not yet the answer to @latest, and a build picks up the version before the
# fix for an hour.
ARG ARU_VERSION=v0.22.0
RUN go install github.com/arandu-io/aru@${ARU_VERSION} && \
    "$(go env GOPATH)/bin/aru" view:build

RUN CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /app .

# Distroless: no shell, no package manager, nothing to exploit that is not the
# application. `docker exec` into it does not work, and that is the trade -- what
# you get instead is an image with no CVEs of its own.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /app /app

# nonroot, from the base image. A container that runs as root is one container
# escape away from being root on the node.
USER nonroot:nonroot

EXPOSE 8080

# The migration is NOT here. `aru migrate` is a pipeline step, before the
# rollout: with N replicas starting together, N migrations race (RULE 16).
ENTRYPOINT ["/app"]
CMD ["serve"]
