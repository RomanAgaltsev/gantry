# The forge is a package in the gantry module, so the build context is the repo
# root: examples/demo/stand/forge/ on its own has no go.mod.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /forge ./examples/demo/stand/forge

FROM gcr.io/distroless/static:nonroot
COPY --from=build /forge /forge
COPY examples/demo/stand/forge/releases.json /releases.json
# TLS cert and key come from the mounted /run/secrets, which is where the shipped
# demo config already points.
ENTRYPOINT ["/forge", "-seed", "/releases.json"]
