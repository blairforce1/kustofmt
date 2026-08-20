# The binary is fully static (CGO_ENABLED=0) and never makes a network call, so
# there is nothing for a base image to provide -- no libc, no CA bundle, no
# shell. scratch is not a stunt here, it is the accurate dependency set.
FROM scratch

# GoReleaser builds every platform up front and stages each one under its own
# $TARGETPLATFORM directory in the build context, so the image never compiles
# anything itself.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/kustofmt /kustofmt

# Numeric UID/GID works without /etc/passwd, which scratch does not have.
USER 65532:65532

ENTRYPOINT ["/kustofmt"]
