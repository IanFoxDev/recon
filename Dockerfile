# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/recon ./cmd/recon

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/recon /recon
WORKDIR /work
USER nonroot:nonroot
ENTRYPOINT ["/recon"]
CMD ["run", "-c", "/work/recon.yaml"]
