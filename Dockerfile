FROM golang:1.16-alpine AS builder

ADD election/src /go/src
RUN cd /go/src \
 && CGO_ENABLED=0 GOOS=linux GO111MODULE=off go build -a -installsuffix cgo -ldflags '-w' -o leader-elector example/main.go

# Regular image
FROM debian:jessie

COPY --from=builder /go/src/leader-elector /usr/bin/

USER 1001

ENTRYPOINT [ "leader-elector" ]

LABEL \
        io.k8s.description="This is a component of OpenShift Container Platform and provides a leader-elector sidecar container." \
        com.redhat.component="leader-elector-container" \
        maintainer="Michal Dulko <mdulko@redhat.com>" \
        name="openshift/ose-leader-elector" \
        summary="This image provides leader election functionality and can be used as a sidecar container." \
        io.k8s.display-name="leader-elector" \
        version="v4.0.0" \
        io.openshift.tags="openshift"
