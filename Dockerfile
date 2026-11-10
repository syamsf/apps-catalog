FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../internal/web/dist && npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/web/dist/ internal/web/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/portal ./cmd/portal

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/portal /portal
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/portal"]
CMD ["serve"]
