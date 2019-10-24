FROM golang:1.13-alpine3.10 as builder
RUN ["mkdir", "-p", "/rio-autoscaler"]
COPY ./ /rio-autoscaler
WORKDIR /rio-autoscaler
RUN ["go", "build", "--mod", "vendor"]
ENTRYPOINT ["./rio-autoscaler"]

FROM alpine:3.9 as production
COPY --from=builder /rio-autoscaler/rio-autoscaler /usr/bin/
ENTRYPOINT ["rio-autoscaler"]