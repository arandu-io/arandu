# One artifact: a static binary in a distroless image.
#
# No runtime, no package manager, no shell. The image is what doc 17 promises
# and what every platform consumes -- Dokploy, Coolify, Kamal, Fly, Railway, ECS
# and Kubernetes all take an OCI image and need nothing else from us.

FROM golang:1.26-alpine AS build

# git for the version stamp, and libstdc++ for the Tailwind binary. There is no
# node, no python and no build toolchain here, which is the point of RULE 13.
#
# libstdc++ is not a toolchain: it is the C++ runtime the standalone tailwindcss
# is linked against, and Alpine ships neither it nor libgcc by default. Two
# errors in a row named this, and the second one is the readable half:
#
#   fork/exec …/tailwindcss-v4.3.3: no such file or directory
#   Error loading shared library libstdc++.so.6: No such file or directory
#
# The first was the glibc build on a musl system -- the loader missing, reported
# as the file missing. `aru` downloads the musl build now, and with the right
# loader the binary can finally say what it actually wants.
#
# No gcompat: that is Alpine's glibc shim, and it would be papering over the
# first error rather than fixing it.
RUN apk add --no-cache git ca-certificates libstdc++

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
ARG ARU_VERSION=v0.76.1
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

# Production unless the deployment says otherwise. The configuration reads a
# missing APP_ENV as production as well, and the line stays so the image does
# not hang on a library default: dev serves /_arandu/debug with every recent
# query and its arguments, prints stack traces on the error page and drops
# Secure from the cookies. A staging deployment sets APP_ENV=staging; nothing
# that runs this image should need dev.
#
# Production asks two things of the deployment at boot: a real mailer, because
# MAIL_URL unset or log:// writes reset codes into the log, and no
# SESSION_SECURE_COOKIE=false. Either one stops the process naming the variable.
ENV APP_ENV=prod

EXPOSE 8080

# The migration is NOT here. `aru migrate` is a pipeline step, before the
# rollout: with N replicas starting together, N migrations race (RULE 16).
ENTRYPOINT ["/app"]
CMD ["serve"]
