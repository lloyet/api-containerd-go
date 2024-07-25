FROM golang:1.21-alpine3.18 as BUILD

WORKDIR /usr/src/app

COPY go.mod go.sum /usr/src/app/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /usr/src/app/build/containerd

FROM scratch

WORKDIR /app

COPY --from=BUILD /usr/src/app/build/containerd /app/containerd

EXPOSE 50051

ENTRYPOINT [ "/app/containerd" ]