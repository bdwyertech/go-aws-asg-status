FROM golang:1.14-alpine as aws-asg-status
WORKDIR /go/src/github.com/bdwyertech/aws-asg-status
COPY . .
ARG VCS_REF
RUN CGO_ENABLED=0 GOFLAGS='-mod=vendor' go build -ldflags="-X main.GitCommit=$VCS_REF -X main.ReleaseVer=docker -X main.ReleaseDate=$BUILD_DATE" .

FROM library/alpine:3.11
COPY --from=better-cfn-signal /go/src/github.com/bdwyertech/aws-asg-status/aws-asg-status /usr/local/bin/

ARG BUILD_DATE
ARG VCS_REF

LABEL org.opencontainers.image.title="bdwyertech/aws-asg-status" \
      org.opencontainers.image.version=$VCS_REF \
      org.opencontainers.image.description="For simplified use of AWS Autoscaling Group Standby functionality" \
      org.opencontainers.image.authors="Brian Dwyer <bdwyertech@github.com>" \
      org.opencontainers.image.url="https://hub.docker.com/r/bdwyertech/aws-asg-status" \
      org.opencontainers.image.source="https://github.com/bdwyertech/aws-asg-status.git" \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_DATE \
      org.label-schema.name="bdwyertech/better-cfn-signal" \
      org.label-schema.description="For simplified use of AWS Autoscaling Group Standby functionality" \
      org.label-schema.url="https://hub.docker.com/r/bdwyertech/aws-asg-status" \
      org.label-schema.vcs-url="https://github.com/bdwyertech/aws-asg-status.git"\
      org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.build-date=$BUILD_DATE

RUN apk update && apk upgrade \
    && apk add --no-cache bash ca-certificates curl jq \
    && adduser aws-asg-status -S -h /home/aws-asg-status

USER aws-asg-status
WORKDIR /home/aws-asg-status
CMD ["bash"]
