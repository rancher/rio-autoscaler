FROM golang AS builder

COPY ./autoscaler /usr/bin
EXPOSE 8080
ENTRYPOINT ["autoscaler"]