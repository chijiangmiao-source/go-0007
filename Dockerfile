FROM --platform=$BUILDPLATFORM golang:1.25.6 AS build
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build ./...
RUN go build -o /out/leo-loop ./cmd/leo-loop

FROM golang:1.25.6
ENV GOTOOLCHAIN=local
WORKDIR /app
COPY --from=build /out/leo-loop /usr/local/bin/leo-loop
COPY --from=build /src/web/src ./web/src
EXPOSE 8080
ENTRYPOINT ["leo-loop"]
CMD ["serve", "-addr", "0.0.0.0:8080", "-store", "/app/data/leo-loop-state.json"]
