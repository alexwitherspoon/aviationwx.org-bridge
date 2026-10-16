#!/bin/sh
set -eu

/mediamtx /mediamtx.yml &
sleep 1

# Publish looping RTSP for cam-b from the first sequence image.
# -loop 1 (not -stream_loop -1) keeps the still streaming; -g 15 emits a
# keyframe each second so readers lock on without waiting for the next GOP.
if [ -f /images/seq-001.jpg ]; then
  ffmpeg -hide_banner -loglevel error -re -loop 1 -framerate 15 -i /images/seq-001.jpg \
    -c:v libx264 -tune zerolatency -g 15 \
    -f rtsp -rtsp_transport tcp "rtsp://127.0.0.1:8554/cam-b" &
fi

exec /usr/local/bin/camera-simulator
