FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/k8s-proxy-api .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /

COPY --from=build /out/k8s-proxy-api /k8s-proxy-api

EXPOSE 8080
ENTRYPOINT ["/k8s-proxy-api"]
