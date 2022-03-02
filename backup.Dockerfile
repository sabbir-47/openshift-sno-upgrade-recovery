FROM registry.ci.openshift.org/openshift/release:golang-1.17 AS builder
ENV GOFLAGS=-mod=mod
WORKDIR /go/src/github.com/redhat-ztp/openshift-SNO-upgrade-recovery
# Bring in the go dependencies before anything else so we can take
# advantage of caching these layers in future builds.
COPY backup-image/go.mod go.mod
COPY backup-image/go.sum go.sum
COPY .git .git
RUN go mod download
COPY backup-image/. .
RUN make build

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /go/src/github.com/redhat-ztp/openshift-SNO-upgrade-recovery/bin/openshift-ai-image-backup /usr/bin/openshift-ai-image-backup
ENTRYPOINT ["/usr/bin/openshift-ai-image-backup"]
