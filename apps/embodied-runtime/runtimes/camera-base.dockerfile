# Camera Runtime base image — provides the runtime dependencies for
# camera-controller. The Go binaries (camera-controller, camctr) are
# injected at deploy time via initContainer / volume mount.
#
# Build:
#   make docker-camera
#   # or
#   docker build -t $(REGISTRY)/camera-base:latest -f runtimes/camera-base.dockerfile .
#
# Usage in a Pod:
#   initContainer copies binaries into a shared emptyDir, then the
#   camera-controller container runs from this image with the binaries
#   mounted at /usr/local/bin.

ARG RUNTIME_BASE_IMAGE=alpine:3.21

FROM ${RUNTIME_BASE_IMAGE}

# Optional: override the Alpine APK mirror (e.g. https://mirrors.tuna.tsinghua.edu.cn).
# When empty, the default CDN (dl-cdn.alpinelinux.org) is used.
ARG APK_MIRROR=""

RUN if [ -n "$APK_MIRROR" ]; then \
      sed -i "s#http://dl-cdn.alpinelinux.org#${APK_MIRROR}#g" /etc/apk/repositories; \
      sed -i "s#https://dl-cdn.alpinelinux.org#${APK_MIRROR}#g" /etc/apk/repositories; \
    fi

# ffmpeg            — the camera driver spawns ffmpeg subprocesses to
#                     capture from V4L2 / RTSP / RealSense devices and
#                     transcode to jpeg / png / bmp / tiff / h264 / h265.
# ca-certificates   — TLS root CAs for HTTPS / RTSP-over-TLS streams.
# tzdata            — timezone data for log timestamps.
RUN apk add --no-cache \
      ffmpeg \
      ca-certificates \
      tzdata
