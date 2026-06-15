# Build a static binary, ship it on scratch. For Unraid, run this container
# with the data shares bind-mounted and --fence pointed at the share root.
#
# NOTE: This image is the file-serving use case (fenced share, no-auth). To manage an Unraid
# HOST itself (reboot-persistent worker with root host access), see deploy/unraid/ instead.
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /sparkyctrl .

FROM scratch
COPY --from=build /sparkyctrl /sparkyctrl
EXPOSE 7766
ENTRYPOINT ["/sparkyctrl", "serve", "--addr", "0.0.0.0:7766", "--no-auth"]
