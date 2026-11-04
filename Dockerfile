FROM golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download && go mod verify

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS=linux
ARG TARGETARCH=amd64

# One image, four binaries: every Deployment picks its service with `command`.
RUN for svc in frontend orders payments reports; do \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -trimpath -buildvcs=false -ldflags="-s -w" -o "/out/${svc}" "./cmd/${svc}" || exit 1; \
    done

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

LABEL org.opencontainers.image.source="https://github.com/ahmedelgmal310/zero-trust-service-mesh" \
      org.opencontainers.image.title="zt-shop" \
      org.opencontainers.image.description="frontend, orders, payments and reports demo services for the zero-trust mesh" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/ /usr/local/bin/
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/frontend"]
