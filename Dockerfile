FROM golang:1.21-alpine3.18 as BUILD

WORKDIR /usr/src/app

COPY . .

RUN go run build -i /usr/src/app/build/containerd

FROM scratch

WORKDIR /app

COPY --from=BUILD /usr/src/app/build/containerd /app/containerd

EXPOSE 50051

ENTRYPOINT [ "/app/containerd" ]