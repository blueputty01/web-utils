FROM golang:1.27-alpine AS build
RUN apk add --no-cache build-base cmake git
RUN git clone --depth 1 https://github.com/ggerganov/whisper.cpp.git /opt/whisper.cpp \
    && cmake -S /opt/whisper.cpp -B /opt/whisper.cpp/build_go -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
    && cmake --build /opt/whisper.cpp/build_go --target whisper -j$(nproc)
ENV C_INCLUDE_PATH=/opt/whisper.cpp/include:/opt/whisper.cpp/ggml/include
ENV LIBRARY_PATH=/opt/whisper.cpp/build_go/src:/opt/whisper.cpp/build_go/ggml/src
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api \
    && CGO_ENABLED=0 go build -o /out/object-store ./cmd/object-store \
    && CGO_ENABLED=1 go build -tags whisper -o /out/worker ./cmd/worker

FROM alpine:3.24.3 AS runtime
RUN apk add --no-cache ca-certificates ffmpeg yt-dlp libstdc++ libgomp \
    && adduser -D -u 10001 app \
    && mkdir -p /work /models /data \
    && chown -R app:app /work /data
USER app
WORKDIR /app
COPY --from=build /out /app/
