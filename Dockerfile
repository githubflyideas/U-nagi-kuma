# syntax=docker/dockerfile:1
# A static binary on an empty image. Build: docker build --build-arg VERSION=v0.1.0 .
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags timetzdata \
      -ldflags "-s -w -X main.version=$VERSION" -o /out/unagikuma ./cmd/unagikuma \
 && mkdir -p /out/data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/unagikuma /unagikuma
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
ENV UNAGIKUMA_DATA=/data
VOLUME /data
EXPOSE 8077
ENTRYPOINT ["/unagikuma"]
CMD ["server"]
